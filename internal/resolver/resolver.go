// Package resolver manages the installer's hosts-file entry for the Naslos
// domain. It only ever touches the single line it marks, so user entries and
// other tools' entries are preserved (docs/installer-contract.md §?).
package resolver

import (
	"fmt"
	"os"
	"strings"
)

// Marker identifies the installer's hosts line.
const Marker = "# naslos-installer"

// Line is the hosts entry for the domain, marked as installer-owned.
func Line(ip, domain string) string {
	return fmt.Sprintf("%s\t%s %s", ip, domain, Marker)
}

// Upsert returns content with the installer's single marked line replaced by a
// line for domain, or appended when absent. Unmarked lines are untouched.
func Upsert(content, ip, domain string) string {
	kept := removeMarked(content)
	kept = append(kept, Line(ip, domain))
	return strings.Join(kept, "\n") + "\n"
}

// Remove returns content without the installer's marked line.
func Remove(content string) string {
	kept := removeMarked(content)
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}

func removeMarked(content string) []string {
	body := strings.TrimRight(content, "\n")
	if body == "" {
		return nil
	}
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, Marker) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

// Install rewrites the hosts file at path with the installer's marked line.
// A permission error is returned so the caller can show the line as a fallback
// instead (elevation is the caller's/OS's concern).
func Install(path, ip, domain string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	next := Upsert(string(content), ip, domain)
	if next == string(content) {
		return nil
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
