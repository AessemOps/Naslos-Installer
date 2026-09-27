package otpauth

import (
	"strings"
	"testing"
)

// 32 bytes of base32 (Authelia's default secret size).
var testSecret = strings.Repeat("JBSWY3DPEHPK3PXP", 3) + "JBSW"

func TestParseGenerateOutput(t *testing.T) {
	out := "Successfully generated TOTP configuration for user 'admin' with URI " +
		"'otpauth://totp/naslos.local:admin?algorithm=SHA1&digits=6&issuer=naslos.local&period=30&secret=" + testSecret + "'"

	cfg, err := ParseGenerateOutput(out)
	if err != nil {
		t.Fatalf("ParseGenerateOutput: %v", err)
	}
	if cfg.Issuer != "naslos.local" || cfg.Account != "admin" {
		t.Fatalf("issuer/account = %q/%q", cfg.Issuer, cfg.Account)
	}
	if cfg.Secret != testSecret {
		t.Fatalf("secret = %q", cfg.Secret)
	}
	if cfg.Algorithm != "SHA1" || cfg.Digits != 6 || cfg.Period != 30 {
		t.Fatalf("params = %+v", cfg)
	}
}

func TestParseGenerateOutputWithoutURI(t *testing.T) {
	if _, err := ParseGenerateOutput("Successfully deleted TOTP configuration for user 'admin'"); err == nil {
		t.Fatal("want error when no otpauth URI is present")
	}
}

func TestParseURIRejectsBadInput(t *testing.T) {
	base := "otpauth://totp/acct?secret=" + testSecret
	cases := map[string]string{
		"wrong scheme":  "https://totp/acct?secret=" + testSecret,
		"wrong type":    "otpauth://hotp/acct?secret=" + testSecret,
		"no secret":     "otpauth://totp/acct",
		"short secret":  "otpauth://totp/acct?secret=JBSWY3DPEHPK3PXP",
		"non-base32":    "otpauth://totp/acct?secret=!!!!",
		"no account":    "otpauth://totp/?secret=" + testSecret,
		"empty account": "otpauth://totp/naslos.local:?secret=" + testSecret,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseURI(raw); err == nil {
				t.Fatalf("want error for %q", raw)
			}
		})
	}
	if _, err := ParseURI(base); err != nil {
		t.Fatalf("valid minimal URI rejected: %v", err)
	}
}

func TestParseURIQueryIssuerWins(t *testing.T) {
	raw := "otpauth://totp/label:acct?issuer=naslos.local&secret=" + testSecret
	cfg, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("ParseURI: %v", err)
	}
	if cfg.Issuer != "naslos.local" || cfg.Account != "acct" {
		t.Fatalf("issuer/account = %q/%q", cfg.Issuer, cfg.Account)
	}
}
