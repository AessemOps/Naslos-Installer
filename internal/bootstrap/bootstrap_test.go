package bootstrap

import (
	"context"
	"strings"
	"testing"
)

type fakeExec struct {
	calls  [][]string
	stdins []string
	reply  func(command []string, stdin string) (string, string, error)
}

func (f *fakeExec) Exec(_ context.Context, _, _, _ string, command []string) (string, string, error) {
	return f.ExecWithStdin(context.Background(), "", "", "", command, "")
}

func (f *fakeExec) ExecWithStdin(_ context.Context, _, _, _ string, command []string, stdin string) (string, string, error) {
	f.calls = append(f.calls, command)
	f.stdins = append(f.stdins, stdin)
	if f.reply != nil {
		return f.reply(command, stdin)
	}
	return "", "", nil
}

func TestCreateAdminKeepsSecretsOffArgv(t *testing.T) {
	gets := 0
	f := &fakeExec{
		reply: func(_ []string, stdin string) (string, string, error) {
			if strings.Contains(stdin, "request = \"POST\"") {
				return `{"uid":"admin"}`, "", nil
			}
			// First the idempotency pre-check, then the post-create check.
			gets++
			if gets == 1 {
				return `[]`, "", nil
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
	if len(f.calls) != 3 {
		t.Fatalf("want pre-check, POST, verify; got %d calls", len(f.calls))
	}
	argv := strings.Join(f.calls[1], " ")
	if argv != "curl -sS --fail-with-body -K -" {
		t.Fatalf("unexpected argv: %s", argv)
	}
	if strings.Contains(argv, "s3cr3t") || strings.Contains(argv, "Correct1") {
		t.Fatalf("a secret leaked into argv: %s", argv)
	}
	post := f.stdins[1]
	for _, want := range []string{
		"header = \"X-Naslos-Proxy-Secret: s3cr3t\"",
		"header = \"Remote-Groups: naslos_admins\"",
		"naslos_admins",
		"admin@naslos.local",
		"Correct1",
		"request = \"POST\"",
	} {
		if !strings.Contains(post, want) {
			t.Fatalf("POST config missing %q:\n%s", want, post)
		}
	}
	if strings.Contains(strings.Join(f.calls[2], " "), "s3cr3t") {
		t.Fatalf("the secret leaked into the GET argv")
	}
}

func TestCreateAdminFailsWhenNotListed(t *testing.T) {
	f := &fakeExec{reply: func([]string, string) (string, string, error) { return `[]`, "", nil }}
	err := CreateAdmin(context.Background(), f, AdminOptions{UID: "admin", Domain: "naslos.local", APIURL: "http://api"})
	if err == nil || !strings.Contains(err.Error(), "not present") {
		t.Fatalf("want a not-present error, got %v", err)
	}
}

// A substring match accepted "administrator" for uid "admin".
func TestCreateAdminRequiresTheExactUID(t *testing.T) {
	f := &fakeExec{reply: func(_ []string, stdin string) (string, string, error) {
		if strings.Contains(stdin, "request = \"POST\"") {
			return `{"uid":"admin"}`, "", nil
		}
		return `[{"uid":"administrator"}]`, "", nil
	}}
	err := CreateAdmin(context.Background(), f, AdminOptions{UID: "admin", Domain: "naslos.local", APIURL: "http://api"})
	if err == nil || !strings.Contains(err.Error(), "not present") {
		t.Fatalf("want a not-present error for a different uid, got %v", err)
	}
}

func TestCurlQuoteEscapesConfigMetacharacters(t *testing.T) {
	got := curlQuote("a\"b\\c\nd")
	want := `"a\"b\\c\nd"`
	if got != want {
		t.Fatalf("curlQuote = %s, want %s", got, want)
	}
}

func TestGenerateTOTP(t *testing.T) {
	const out = "Successfully generated TOTP configuration for user 'admin' with URI 'otpauth://totp/naslos.local:admin?algorithm=SHA1&digits=6&issuer=naslos.local&period=30&secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXPJBSWY3DP'"
	f := &fakeExec{reply: func([]string, string) (string, string, error) { return out, "", nil }}
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

func TestCreateAdminIsIdempotentWhenTheUserAlreadyExists(t *testing.T) {
	f := &fakeExec{reply: func(_ []string, stdin string) (string, string, error) {
		if strings.Contains(stdin, "request = \"POST\"") {
			t.Fatal("an existing administrator must not be POSTed again")
		}
		return `[{"uid":"admin"}]`, "", nil
	}}
	err := CreateAdmin(context.Background(), f, AdminOptions{
		UID: "admin", Domain: "naslos.local", Password: "Correct1",
		ProxySecret: "s3cr3t", APIURL: "http://api",
		TerminalNamespace: "naslos", TerminalPod: "p", TerminalContainer: "c",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("want only the existence check, got %d calls", len(f.calls))
	}
}
