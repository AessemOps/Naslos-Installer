package talosclient

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAPI struct {
	versionCalls int
	versionErr   error
	bootstrapErr error
	bootstrapN   int
}

func (f *fakeAPI) Apply(context.Context, []byte) error { return nil }
func (f *fakeAPI) Bootstrap(context.Context) error {
	f.bootstrapN++
	return f.bootstrapErr
}
func (f *fakeAPI) Kubeconfig(context.Context) ([]byte, error) { return []byte("kubeconfig"), nil }
func (f *fakeAPI) Version(context.Context) (string, error) {
	f.versionCalls++
	return "v1.14.1", f.versionErr
}
func (f *fakeAPI) Services(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (f *fakeAPI) Close() error { return nil }

func TestWaitForAPIReturnsOnFirstSuccess(t *testing.T) {
	f := &fakeAPI{}
	if err := WaitForAPI(context.Background(), f, time.Second); err != nil {
		t.Fatalf("WaitForAPI: %v", err)
	}
	if f.versionCalls != 1 {
		t.Fatalf("version calls = %d, want 1", f.versionCalls)
	}
}

func TestWaitForAPITimesOut(t *testing.T) {
	f := &fakeAPI{versionErr: errors.New("connection refused")}
	err := WaitForAPI(context.Background(), f, 50*time.Millisecond)
	if err == nil {
		t.Fatal("want timeout error")
	}
}

func TestBootstrapTreatsAlreadyBootstrappedAsSuccess(t *testing.T) {
	for _, msg := range []string{
		"bootstrap is already done",
		"etcd is already bootstrapped",
		"rpc error: code = AlreadyExists desc = member already exists",
	} {
		f := &fakeAPI{bootstrapErr: errors.New(msg)}
		if err := Bootstrap(context.Background(), f); err != nil {
			t.Fatalf("Bootstrap(%q) = %v, want nil", msg, err)
		}
	}
}

func TestIsAlreadyBootstrappedDoesNotSwallowAuthErrors(t *testing.T) {
	// "unknown authority" means the wrong PKI, not an already-bootstrapped
	// cluster; swallowing it would hide a broken install.
	if IsAlreadyBootstrapped(errors.New("certificate signed by unknown authority")) {
		t.Fatal("unknown-authority must not be treated as already-bootstrapped")
	}
}

func TestBootstrapPropagatesRealErrors(t *testing.T) {
	f := &fakeAPI{bootstrapErr: errors.New("connection refused")}
	if err := Bootstrap(context.Background(), f); err == nil {
		t.Fatal("want the real error to propagate")
	}
	if f.bootstrapN != 1 {
		t.Fatalf("bootstrap attempts = %d, want 1", f.bootstrapN)
	}
}

func TestIsAlreadyBootstrapped(t *testing.T) {
	if IsAlreadyBootstrapped(nil) {
		t.Fatal("nil is not already-bootstrapped")
	}
	if IsAlreadyBootstrapped(errors.New("boom")) {
		t.Fatal("unrelated error is not already-bootstrapped")
	}
}
