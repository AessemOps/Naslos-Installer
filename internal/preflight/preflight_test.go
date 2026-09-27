package preflight

import (
	"net"
	"strconv"
	"testing"
	"time"
)

func listen(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestCheckClassifiesNode(t *testing.T) {
	api := listen(t)
	k8s := listen(t)

	state, err := check("127.0.0.1", api, k8s, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if state != Installed {
		t.Fatalf("want installed (k8s port up), got %q", state)
	}

	state, err = check("127.0.0.1", api, freePort(t), 500*time.Millisecond)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if state != Maintenance {
		t.Fatalf("want maintenance (only api port up), got %q", state)
	}
}

func TestCheckUnreachable(t *testing.T) {
	state, err := check("127.0.0.1", freePort(t), freePort(t), 200*time.Millisecond)
	if err == nil {
		t.Fatal("want error when nothing is listening")
	}
	if state != Unreachable {
		t.Fatalf("want unreachable, got %q", state)
	}
}

func TestCheckEmptyIP(t *testing.T) {
	if _, err := check("", APIPort, K8sPort, time.Second); err == nil {
		t.Fatal("want error for empty IP")
	}
}

func TestDialToListener(t *testing.T) {
	port := listen(t)
	if err := Dial(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second); err != nil {
		t.Fatalf("Dial: %v", err)
	}
}
