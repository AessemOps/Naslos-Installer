// Package bootstrap creates the first administrator through the owner-gated API
// and generates the administrator's TOTP device, per
// docs/installer-contract.md §3–4 (FR-INSTALL-06/07).
package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AessemOps/Naslos-Installer/internal/otpauth"
)

// Execer runs a command in a pod container. *k8s.Client implements it.
type Execer interface {
	Exec(ctx context.Context, namespace, pod, container string, command []string) (string, string, error)
	ExecWithStdin(ctx context.Context, namespace, pod, container string, command []string, stdin string) (string, string, error)
}

// AdminOptions describe the first administrator and the terminal exec target.
type AdminOptions struct {
	UID               string
	Domain            string
	Password          string
	ProxySecret       string
	TerminalNamespace string
	TerminalPod       string
	TerminalContainer string
	APIURL            string
}

// CreateAdmin POSTs the administrator to /api/users from the terminal pod and
// verifies it is listed. The API is owner-gated, so the proxy secret and the
// owner headers are required.
func CreateAdmin(ctx context.Context, e Execer, o AdminOptions) error {
	body, err := json.Marshal(map[string]interface{}{
		"uid":       o.UID,
		"firstName": o.UID,
		"lastName":  "Admin",
		"email":     fmt.Sprintf("%s@%s", o.UID, o.Domain),
		"password":  o.Password,
		"groups":    []string{"naslos_admins"},
	})
	if err != nil {
		return err
	}

	// A retry (or a resume after the admin step) must not POST a second time.
	// If the exact uid is already listed, this work is done.
	if existing, err := users(ctx, e, o); err == nil {
		if present, parseErr := hasUID(existing, o.UID); parseErr == nil && present {
			return nil
		}
	}

	api := strings.TrimRight(o.APIURL, "/")

	// The password and proxy secret go through curl's config on stdin. Putting
	// them in argv put both into the kube-apiserver audit log (the default
	// policy records request URIs at Metadata level) and into /proc.
	config := "url = " + curlQuote(api+"/api/users") + "\n" +
		"request = \"POST\"\n" +
		curlHeader("X-Naslos-Proxy-Secret", o.ProxySecret) +
		curlHeader("Remote-User", o.UID) +
		curlHeader("Remote-Groups", "naslos_admins") +
		curlHeader("Content-Type", "application/json") +
		"data-binary = " + curlQuote(string(body)) + "\n"
	stdout, stderr, err := e.ExecWithStdin(
		ctx, o.TerminalNamespace, o.TerminalPod, o.TerminalContainer,
		[]string{"curl", "-sS", "--fail-with-body", "-K", "-"}, config,
	)
	if err != nil {
		return fmt.Errorf("creating the administrator: %w (response: %s)", err, strings.TrimSpace(stdout+" "+stderr))
	}

	list, listErr := users(ctx, e, o)
	if listErr != nil {
		return listErr
	}
	found, err := hasUID(list, o.UID)
	if err != nil {
		return fmt.Errorf("parsing the user list: %w (response: %s)", err, strings.TrimSpace(list))
	}
	if !found {
		return fmt.Errorf("administrator %q is not present after creation (create said %q; list %q)",
			o.UID, strings.TrimSpace(stdout), strings.TrimSpace(list))
	}
	return nil
}

// hasUID reports whether the JSON user list contains an entry whose uid is
// exactly uid. A substring check accepted "administrator" for "admin".
func hasUID(list, uid string) (bool, error) {
	var people []struct {
		UID string `json:"uid"`
	}
	if err := json.Unmarshal([]byte(list), &people); err != nil {
		return false, err
	}
	for _, person := range people {
		if person.UID == uid {
			return true, nil
		}
	}
	return false, nil
}

// curlQuote quotes a value for a curl config file, which uses C-style
// backslash escapes inside double quotes.
func curlQuote(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(value) + `"`
}

func curlHeader(name, value string) string {
	return "header = " + curlQuote(name+": "+value) + "\n"
}

func users(ctx context.Context, e Execer, o AdminOptions) (string, error) {
	config := "url = " + curlQuote(strings.TrimRight(o.APIURL, "/")+"/api/users") + "\n" +
		curlHeader("X-Naslos-Proxy-Secret", o.ProxySecret) +
		curlHeader("Remote-User", o.UID) +
		curlHeader("Remote-Groups", "naslos_admins")
	stdout, stderr, err := e.ExecWithStdin(
		ctx, o.TerminalNamespace, o.TerminalPod, o.TerminalContainer,
		[]string{"curl", "-sS", "--fail-with-body", "-K", "-"}, config,
	)
	if err != nil {
		return "", fmt.Errorf("listing users: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// GenerateTOTP runs `authelia storage user totp generate` in the Authelia pod
// and parses the printed otpauth URI. The binary is exec'd directly (the image
// is distroless) and the config/DB never leave the pod.
func GenerateTOTP(ctx context.Context, e Execer, namespace, pod, container, uid, domain string) (*otpauth.Config, error) {
	stdout, stderr, err := e.Exec(ctx, namespace, pod, container, []string{
		"authelia", "storage", "user", "totp", "generate", uid, "--issuer", domain,
	})
	if err != nil {
		return nil, fmt.Errorf("running authelia totp generate: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	cfg, err := otpauth.ParseGenerateOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parsing the TOTP output: %w (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	return cfg, nil
}
