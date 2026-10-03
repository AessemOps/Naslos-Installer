package helm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeValuesPrecedence(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "values.yaml")
	installer := filepath.Join(dir, "values-installer.yaml")
	if err := os.WriteFile(base, []byte("a: base\nb: base\nnested:\n  x: base\n  w: base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installer, []byte("b: installer\nnested:\n  w: installer\n  z: installer\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := MergeValues([]string{base, installer}, map[string]interface{}{
		"nested": map[string]interface{}{"z": "override"},
	})
	if err != nil {
		t.Fatalf("MergeValues: %v", err)
	}

	nested, ok := got["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested is %T", got["nested"])
	}
	cases := []struct{ path, want string }{
		{"a", "base"},
		{"b", "installer"},
	}
	for _, c := range cases {
		if got[c.path] != c.want {
			t.Errorf("%s = %v, want %s", c.path, got[c.path], c.want)
		}
	}
	for key, want := range map[string]string{"x": "base", "w": "installer", "z": "override"} {
		if nested[key] != want {
			t.Errorf("nested.%s = %v, want %s", key, nested[key], want)
		}
	}
}

func TestOverrides(t *testing.T) {
	got := Overrides("naslos.example", "naslos", "192.168.1.0/24", "naslos-talosconfig")

	if got["domain"] != "naslos.example" {
		t.Errorf("domain = %v", got["domain"])
	}
	openldap := got["openldap"].(map[string]interface{})
	if openldap["host"] != "ldap.naslos.example" {
		t.Errorf("openldap.host = %v", openldap["host"])
	}
	sso := got["sso"].(map[string]interface{})
	if domains, ok := sso["domains"].([]interface{}); !ok || len(domains) != 1 || domains[0] != "naslos.example" {
		t.Errorf("sso.domains = %v", sso["domains"])
	}
	shares := got["shares"].(map[string]interface{})
	discovery := shares["discovery"].(map[string]interface{})
	if discovery["name"] != "naslos" {
		t.Errorf("shares.discovery.name = %v", discovery["name"])
	}
	api := got["api"].(map[string]interface{})
	if api["talosConfigSecret"] != "naslos-talosconfig" {
		t.Errorf("api.talosConfigSecret = %v", api["talosConfigSecret"])
	}
	np := got["networkPolicy"].(map[string]interface{})
	for _, key := range []string{"nodeCIDR", "ingressPluginsCIDR", "nfsClientCIDR"} {
		if np[key] != "192.168.1.0/24" {
			t.Errorf("networkPolicy.%s = %v", key, np[key])
		}
	}
}
