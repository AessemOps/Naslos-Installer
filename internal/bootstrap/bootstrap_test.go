package bootstrap

import (
	"context"
	"strings"
	"testing"
)

type fakeExec struct {
	calls [][]string
	reply func(command []string) (string, string, error)
}

func (f *fakeExec) Exec(_ context.Context, _, _, _ string, command []string) (string, string, error) {
	f.calls = append(f.calls, command)
	if f.reply != nil {
		return f.reply(command)
	}
	return "", "", nil
}

func TestCreateAdminPostsThenVerifies(t *testing.T) {
	f := &fakeExec{
		reply: func(command []string) (string, string, error) {
			if strings.Contains(strings.Join(command, " "), "-X POST") {
				return `{"uid":"admin"}`, "", nil
			}
			return `[{"uid":"admin","groups":["naslos_admins"]}]`, "", nil
		},
	}
	err := CreateAdmin(context.Background(), f, AdminOptions{
		UID: "admin", Domain: "naslos.local", Password: "Correct1",
		ProxySecret: "s3cr3t", APIURL: "http://naslos-api.naslos.svc.cluster.local:8080",
		TerminalNamespace: "naslos-privileged", TerminalPod: "naslos-terminal-x", TerminalContainer: "shell",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("want a POST then a GET, got %d calls", len(f.calls))
	}
	post := strings.Join(f.calls[0], " ")
	if !strings.Contains(post, "-X POST") || !strings.Contains(post, "/api/users") {
		t.Fatalf("unexpected POST command: %s", post)
	}
	if !strings.Contains(post, "X-Naslos-Proxy-Secret: s3cr3t") || !strings.Contains(post, "Remote-Groups: naslos_admins") {
		t.Fatalf("POST is missing owner headers: %s", post)
	}
	if !strings.Contains(post, `"groups":["naslos_admins"]`) || !strings.Contains(post, `"email":"admin@naslos.local"`) {
		t.Fatalf("POST body is wrong: %s", post)
	}
	if !strings.HasPrefix(strings.Join(f.calls[1], " "), "curl -sS http") {
		t.Fatalf("GET command should fetch /api/users: %s", strings.Join(f.calls[1], " "))
	}
}

func TestCreateAdminFailsWhenNotListed(t *testing.T) {
	f := &fakeExec{reply: func([]string) (string, string, error) { return `[]`, "", nil }}
	err := CreateAdmin(context.Background(), f, AdminOptions{UID: "admin", Domain: "naslos.local", APIURL: "http://api"})
	if err == nil || !strings.Contains(err.Error(), "not present") {
		t.Fatalf("want a not-present error, got %v", err)
	}
}

func TestGenerateTOTP(t *testing.T) {
	const out = "Successfully generated TOTP configuration for user 'admin' with URI 'otpauth://totp/naslos.local:admin?algorithm=SHA1&digits=6&issuer=naslos.local&period=30&secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXPJBSWY3DP'"
	f := &fakeExec{reply: func([]string) (string, string, error) { return out, "", nil }}
	cfg, err := GenerateTOTP(context.Background(), f, "naslos", "naslos-authelia-0", "authelia", "admin", "naslos.local")
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}
	if cfg.Secret != "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXPJBSWY3DP" {
		t.Fatalf("secret = %q", cfg.Secret)
	}
	command := strings.Join(f.calls[0], " ")
	if !strings.Contains(command, "authelia storage user totp generate admin --issuer naslos.local") {
		t.Fatalf("unexpected command: %s", command)
	}
}
