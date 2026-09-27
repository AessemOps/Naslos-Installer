// Package config holds the installer inputs and their validation.
//
// The engine gathers these from the wizard (or CLI flags), validates them
// before doing anything to the node, and derives the values the chart and the
// machine-config template need.
package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Input is the set of user-provided install inputs.
type Input struct {
	NodeIP        string
	Domain        string
	Name          string // advertised SMB/mDNS name; defaults to the domain's first label
	AdminUser     string
	AdminPassword string
	AddResolver   bool
}

var (
	labelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	uidRE   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{0,31}$`)
	nameRE  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,14}$`)
)

// Validate checks every input before the engine touches the node.
func (in Input) Validate() error {
	if net.ParseIP(in.NodeIP) == nil || net.ParseIP(in.NodeIP).To4() == nil {
		return fmt.Errorf("node IP %q is not a valid IPv4 address", in.NodeIP)
	}
	if in.Domain == "" {
		return errors.New("domain is required")
	}
	if strings.ContainsAny(in.Domain, " \t/:") {
		return fmt.Errorf("domain %q must be a bare hostname", in.Domain)
	}
	for _, label := range strings.Split(strings.ToLower(in.Domain), ".") {
		if !labelRE.MatchString(label) {
			return fmt.Errorf("domain %q has an invalid label %q", in.Domain, label)
		}
	}
	if !uidRE.MatchString(in.AdminUser) {
		return fmt.Errorf("admin user %q must start with a letter and contain only letters, digits, . _ -", in.AdminUser)
	}
	if err := ValidatePassword(in.AdminPassword); err != nil {
		return err
	}
	name := in.ShortName()
	if name == "" || !nameRE.MatchString(name) {
		return fmt.Errorf("advertised name %q is not a valid NetBIOS name (max 15, letters/digits/-)", name)
	}
	return nil
}

// ValidatePassword enforces Authelia's password policy.
func ValidatePassword(pw string) error {
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(pw) > 72 {
		return errors.New("password must be at most 72 characters")
	}
	var upper, lower, digit bool
	for _, r := range pw {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	if !upper || !lower || !digit {
		return errors.New("password must contain an uppercase letter, a lowercase letter and a digit")
	}
	return nil
}

// ShortName is the advertised name: the explicit Name, else the domain's first
// label (so `naslos.local` -> `naslos`). It is what feeds SMB_DISCOVERY_NAME and
// SMB_NETBIOS_NAME, and it must equal the Samba NetBIOS name (FR-SHR-08/10).
func (in Input) ShortName() string {
	if in.Name != "" {
		return in.Name
	}
	first := strings.SplitN(strings.ToLower(in.Domain), ".", 2)[0]
	if len(first) > 15 {
		first = first[:15]
	}
	return first
}

// NormalizedDomain returns the domain lowercased.
func (in Input) NormalizedDomain() string { return strings.ToLower(in.Domain) }

// Subnet24 is the node's IPv4 /24 expressed as a CIDR, e.g. 192.168.1.0/24. The
// engine sets networkPolicy.nodeCIDR and the KubeNodeConfig validSubnets from it.
func (in Input) Subnet24() (string, error) {
	ip := net.ParseIP(in.NodeIP)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("node IP %q is not valid IPv4", in.NodeIP)
	}
	v4 := ip.To4()
	return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2]), nil
}

// AutheliaURL is the first-login URL shown to the user.
func (in Input) AutheliaURL() string {
	return "https://" + in.NormalizedDomain() + "/authelia"
}
