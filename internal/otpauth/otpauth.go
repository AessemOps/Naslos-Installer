// Package otpauth parses the TOTP enrolment the engine shows the user.
//
// The engine runs `authelia storage user totp generate <uid> --issuer <domain>`
// in the Authelia pod; Authelia prints a line containing an otpauth:// URI and
// the shared secret. This package extracts and validates that URI so the shell
// can render a QR and the base32 secret.
package otpauth

import (
	"encoding/base32"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Config is a parsed TOTP enrolment.
type Config struct {
	URI       string
	Type      string
	Issuer    string
	Account   string
	Secret    string
	Algorithm string
	Digits    int
	Period    int
}

var generateRE = regexp.MustCompile(`URI '([^']+)'`)

// ParseGenerateOutput extracts the otpauth URI from the Authelia CLI output,
// e.g. "Successfully generated TOTP configuration for user 'admin' with URI
// 'otpauth://totp/naslos.local:admin?...&secret=...'".
func ParseGenerateOutput(out string) (*Config, error) {
	m := generateRE.FindStringSubmatch(out)
	if m == nil {
		return nil, errors.New("authelia output contains no otpauth URI")
	}
	return ParseURI(m[1])
}

// ParseURI parses and validates an otpauth://totp URI.
func ParseURI(raw string) (*Config, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing otpauth URI: %w", err)
	}
	if u.Scheme != "otpauth" {
		return nil, fmt.Errorf("not an otpauth URI (scheme %q)", u.Scheme)
	}
	if u.Host != "totp" {
		return nil, fmt.Errorf("unsupported otpauth type %q (only totp)", u.Host)
	}

	label := strings.TrimPrefix(u.Path, "/")
	if label == "" {
		return nil, errors.New("otpauth URI has no account label")
	}
	issuer, account := "", label
	if i := strings.LastIndex(label, ":"); i >= 0 {
		issuer, account = label[:i], label[i+1:]
	}
	q := u.Query()
	if v := q.Get("issuer"); v != "" {
		issuer = v
	}
	if account == "" {
		return nil, errors.New("otpauth URI has no account")
	}

	secret := q.Get("secret")
	if secret == "" {
		return nil, errors.New("otpauth URI has no secret")
	}
	if err := validateSecret(secret); err != nil {
		return nil, err
	}

	return &Config{
		URI:       raw,
		Type:      "totp",
		Issuer:    issuer,
		Account:   account,
		Secret:    strings.ToUpper(secret),
		Algorithm: upperDefault(q.Get("algorithm"), "SHA1"),
		Digits:    atoiDefault(q.Get("digits"), 6),
		Period:    atoiDefault(q.Get("period"), 30),
	}, nil
}

// validateSecret enforces Authelia's rule: base32 and more than 20 decoded bytes.
func validateSecret(secret string) error {
	s := strings.ToUpper(strings.TrimRight(secret, "="))
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z') && !(r >= '2' && r <= '7') {
			return fmt.Errorf("otpauth secret is not valid base32")
		}
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return fmt.Errorf("otpauth secret is not valid base32: %w", err)
	}
	if len(decoded) <= 20 {
		return fmt.Errorf("otpauth secret decodes to %d bytes, Authelia requires more than 20", len(decoded))
	}
	return nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func upperDefault(s, def string) string {
	if s == "" {
		return def
	}
	return strings.ToUpper(s)
}
