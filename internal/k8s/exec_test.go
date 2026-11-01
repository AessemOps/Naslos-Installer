package k8s

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// A typed-nil reader is a non-nil io.Reader. It made client-go open a stdin
// stream the kubelet did not expect, which failed with "Timeout occurred".
func TestStreamOptionsLeavesStdinNilWithoutInput(t *testing.T) {
	var out, errOut bytes.Buffer
	if opts := streamOptions("", &out, &errOut); opts.Stdin != nil {
		t.Fatalf("Stdin must be a nil interface when there is no input, got %T", opts.Stdin)
	}
	if opts := streamOptions("payload", &out, &errOut); opts.Stdin == nil {
		t.Fatal("Stdin must be set when there is input")
	}
}

// TestLiveExecWithoutStdin runs only against a real cluster:
//
//	NASLOS_LIVE_KUBECONFIG=~/.config/naslos-install/kubeconfig go test ./internal/k8s -run Live
func TestLiveExecWithoutStdin(t *testing.T) {
	path := os.Getenv("NASLOS_LIVE_KUBECONFIG")
	if path == "" {
		t.Skip("set NASLOS_LIVE_KUBECONFIG to exercise a real kubelet")
	}
	kc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFromKubeconfig(kc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stdout, stderr, err := c.Exec(ctx, "naslos", "naslos-authelia-0", "authelia", []string{"authelia", "--version"})
	if err != nil {
		t.Fatalf("exec without stdin: %v (stderr %q)", err, stderr)
	}
	if !strings.Contains(stdout, "authelia") {
		t.Fatalf("unexpected output %q", stdout)
	}
}
