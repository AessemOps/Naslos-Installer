package talosconfig

import (
	"path/filepath"
	"strings"
	"testing"
)

const testPatch = `machine:
  install:
    image: factory.talos.dev/installer/aaaaaaaa:v1.14.1
    disk: /dev/vda
---
apiVersion: v1alpha1
kind: KubeFlannelCNIConfig
$patch: delete
`

func inputs() Inputs {
	return Inputs{
		ClusterName:    "naslos",
		NodeIP:         "192.168.1.117",
		TalosVersion:   "v1.14.1",
		InstallerImage: "factory.talos.dev/installer/aaaaaaaa:v1.14.1",
		InstallDisk:    "/dev/vda",
	}
}

func TestGenerateAppliesPatch(t *testing.T) {
	dir := t.TempDir()
	bundle, reused, err := LoadOrCreateBundle(filepath.Join(dir, "secrets.json"), "v1.14.1")
	if err != nil {
		t.Fatalf("LoadOrCreateBundle: %v", err)
	}
	if reused {
		t.Fatal("first call should create the bundle")
	}

	res, err := Generate(inputs(), []byte(testPatch), bundle)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	cp := string(res.ControlPlane)
	if !strings.Contains(cp, "factory.talos.dev/installer/aaaaaaaa:v1.14.1") {
		t.Fatalf("patched install image missing from control-plane config")
	}
	if strings.Contains(cp, "UnattendedInstallConfig") {
		t.Fatalf("UnattendedInstallConfig must be skipped")
	}
	if tc := string(res.Talosconfig); !strings.Contains(tc, "192.168.1.117") {
		t.Fatalf("talosconfig does not reference the node endpoint:\n%s", tc)
	}
}

func TestLoadOrCreateBundleReusesPKI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	first, _, err := LoadOrCreateBundle(path, "v1.14.1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, reused, err := LoadOrCreateBundle(path, "v1.14.1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reused {
		t.Fatal("second call must reuse the persisted bundle")
	}
	if string(first.Certs.K8s.Key) != string(second.Certs.K8s.Key) {
		t.Fatal("reloading must not regenerate the PKI")
	}
}

func TestGenerateRequiresNodeIP(t *testing.T) {
	dir := t.TempDir()
	bundle, _, err := LoadOrCreateBundle(filepath.Join(dir, "secrets.json"), "v1.14.1")
	if err != nil {
		t.Fatal(err)
	}
	in := inputs()
	in.NodeIP = ""
	if _, err := Generate(in, []byte(testPatch), bundle); err == nil {
		t.Fatal("want error for a missing node IP")
	}
}
