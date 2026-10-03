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

	api := strings.TrimRight(o.APIURL, "/")
	stdout, stderr, err := e.Exec(ctx, o.TerminalNamespace, o.TerminalPod, o.TerminalContainer, []string{
		"curl", "-sS", "-X", "POST", api + "/api/users",
		"-H", "X-Naslos-Proxy-Secret: " + o.ProxySecret,
		"-H", "Remote-User: " + o.UID,
		"-H", "Remote-Groups: naslos_admins",
		"-H", "Content-Type: application/json",
		"-d", string(body),
	})
	if err != nil {
		return fmt.Errorf("creating the administrator: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	list, listErr := users(ctx, e, o)
	if listErr != nil {
		return listErr
	}
	if !strings.Contains(list, o.UID) {
		return fmt.Errorf("administrator %q is not present after creation (create said %q; list %q)",
			o.UID, strings.TrimSpace(stdout), strings.TrimSpace(list))
	}
	return nil
}

func users(ctx context.Context, e Execer, o AdminOptions) (string, error) {
	stdout, stderr, err := e.Exec(ctx, o.TerminalNamespace, o.TerminalPod, o.TerminalContainer, []string{
		"curl", "-sS", strings.TrimRight(o.APIURL, "/") + "/api/users",
		"-H", "X-Naslos-Proxy-Secret: " + o.ProxySecret,
		"-H", "Remote-User: " + o.UID,
		"-H", "Remote-Groups: naslos_admins",
	})
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
