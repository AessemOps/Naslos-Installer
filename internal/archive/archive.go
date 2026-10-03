// Package archive builds the recovery ZIP the installer leaves behind.
//
// It contains the Talos credentials (talosconfig, controlplane, secrets bundle),
// the kubeconfig, the schematic, the ISO link and a README with the recovery
// steps, admin username, TOTP issuer and login URL. It NEVER contains the admin
// password, and the README warns that the node's buddy-identity.json is the
// backup KEK (docs/installer-contract.md).
package archive

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options describe the recovery archive.
type Options struct {
	Dir           string // output directory
	StateDir      string // where the engine persisted the credentials
	Domain        string
	AdminUser     string
	LoginURL      string
	NaslosVersion string
	TalosVersion  string
	SchematicID   string
	ISOURL        string
	ReadMember    func(name string) ([]byte, error) // pack reader, for schematic/naslos.yaml
	Now           time.Time
}

// stateMembers are copied verbatim from the state directory into the ZIP.
var stateMembers = []string{
	"talosconfig",
	"controlplane.yaml",
	"talos-secrets.json",
	"kubeconfig",
}

// Build writes the recovery ZIP and returns its path.
func Build(o Options) (string, error) {
	if o.Now.IsZero() {
		o.Now = time.Now().UTC()
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return "", fmt.Errorf("creating the output directory: %w", err)
	}
	name := fmt.Sprintf("naslos-recovery-%s-%s.zip", o.Domain, o.Now.Format("20060102T150405Z"))
	path := filepath.Join(o.Dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	write := func(name string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: o.Now})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	for _, member := range stateMembers {
		data, err := os.ReadFile(filepath.Join(o.StateDir, member))
		if err != nil {
			return "", fmt.Errorf("reading state member %s: %w", member, err)
		}
		if err := write(member, data); err != nil {
			return "", err
		}
	}

	if o.ReadMember != nil {
		if data, err := o.ReadMember("schematic/naslos.yaml"); err == nil {
			if err := write("schematic/naslos.yaml", data); err != nil {
				return "", err
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("reading the schematic: %w", err)
		}
	}

	if err := write("ISO.md", []byte(isoDoc(o))); err != nil {
		return "", err
	}
	if err := write("README.txt", []byte(readme(o))); err != nil {
		return "", err
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("finalising %s: %w", path, err)
	}
	return path, nil
}

func isoDoc(o Options) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Recovery ISO")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Naslos version: %s\n", o.NaslosVersion)
	fmt.Fprintf(&b, "- Talos version:  %s\n", o.TalosVersion)
	fmt.Fprintf(&b, "- Schematic:      %s\n", o.SchematicID)
	fmt.Fprintf(&b, "- ISO URL:        %s\n", o.ISOURL)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Boot a replacement machine from this ISO (factory.talos.dev builds it")
	fmt.Fprintln(&b, "from the schematic above) and re-run the installer, or use the Talos")
	fmt.Fprintln(&b, "credentials in this archive with talosctl.")
	return b.String()
}

func readme(o Options) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Naslos recovery bundle")
	fmt.Fprintln(&b, "======================")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "MASTER CREDENTIAL - store offline, never share.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Login URL:   %s\n", o.LoginURL)
	fmt.Fprintf(&b, "Admin user:  %s\n", o.AdminUser)
	fmt.Fprintf(&b, "TOTP issuer: %s\n", o.Domain)
	fmt.Fprintf(&b, "Naslos:      %s\n", o.NaslosVersion)
	fmt.Fprintf(&b, "Talos:       %s\n", o.TalosVersion)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Contents:")
	fmt.Fprintln(&b, "  talosconfig, controlplane.yaml, talos-secrets.json - Talos credentials")
	fmt.Fprintln(&b, "  kubeconfig                                        - cluster admin access")
	fmt.Fprintln(&b, "  schematic/naslos.yaml                             - exact image definition")
	fmt.Fprintln(&b, "  ISO.md                                            - replacement ISO link")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "The administrator password is deliberately NOT included.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "IMPORTANT: on the node, /var/lib/naslos/buddy-identity.json is the")
	fmt.Fprintln(&b, "Buddy Backup key-encryption-key. If it is lost, every stored backup")
	fmt.Fprintln(&b, "becomes unreadable. Back it up separately and keep it with this ZIP.")
	fmt.Fprintln(&b, "(The naslos namespace is helm.sh/resource-policy: keep, so a release")
	fmt.Fprintln(&b, "uninstall does not delete it; deleting the namespace does.)")
	return b.String()
}
