// Package talosclient drives a Talos node through maintenance, install and
// bootstrap.
//
// The engine talks to a freshly-booted node over the Talos API (:50000) with no
// PKI yet (maintenance mode), applies the control-plane config, waits for the
// API to return, and bootstraps etcd. It defines every argument itself and never
// shells out to talosctl (SEC-1).
package talosclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/block"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	"google.golang.org/grpc/codes"
)

// API is the subset of the Talos client the engine uses. It is an interface so
// the lifecycle logic can be tested without a node.
type API interface {
	Apply(ctx context.Context, config []byte) error
	Bootstrap(ctx context.Context) error
	Kubeconfig(ctx context.Context) ([]byte, error)
	Version(ctx context.Context) (string, error)
	Disks(ctx context.Context) ([]DiskInfo, error)
	Interface(ctx context.Context, ip string) (string, error)
	Services(ctx context.Context) (map[string]string, error)
	Close() error
}

// Dial opens a maintenance-mode client for node (no PKI, TLS not verified — the
// node has no certificate to trust yet). Only used before the first apply.
func Dial(ctx context.Context, node string) (API, error) {
	c, err := client.New(ctx,
		client.WithEndpoints(node),
		client.WithTLSConfig(&tls.Config{
			//nolint:gosec // maintenance mode: the node presents a self-signed
			// cert that the installer cannot know in advance (same as
			// `talosctl apply-config --insecure`).
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("opening a Talos client to %s: %w", node, err)
	}
	return &talosAPI{c: c}, nil
}

// DiskInfo is a whole disk reported by the maintenance-mode block subsystem.
// Only fields an operator needs to identify the install target are exposed.
type DiskInfo struct {
	Device     string `json:"device"`
	Size       uint64 `json:"size"`
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	Transport  string `json:"transport,omitempty"`
	Rotational bool   `json:"rotational"`
	Readonly   bool   `json:"readonly"`
	CDROM      bool   `json:"cdrom"`
}

type talosAPI struct{ c *client.Client }

// DialAuthenticated opens a client from a generated talosconfig (endpoints +
// client certificate). Use it after the config has been applied and the node
// has rebooted into the installed system; the maintenance client can no longer
// authenticate then.
func DialAuthenticated(ctx context.Context, tc *clientconfig.Config) (API, error) {
	c, err := client.New(ctx, client.WithConfig(tc))
	if err != nil {
		return nil, fmt.Errorf("opening an authenticated Talos client: %w", err)
	}
	return &talosAPI{c: c}, nil
}

func (t *talosAPI) Apply(ctx context.Context, config []byte) error {
	_, err := t.c.ApplyConfiguration(ctx, &machine.ApplyConfigurationRequest{
		Data: config,
		Mode: machine.ApplyConfigurationRequest_REBOOT,
	})
	return err
}

func (t *talosAPI) Bootstrap(ctx context.Context) error {
	return t.c.Bootstrap(ctx, &machine.BootstrapRequest{})
}

func (t *talosAPI) Kubeconfig(ctx context.Context) ([]byte, error) {
	return t.c.Kubeconfig(ctx)
}

func (t *talosAPI) Version(ctx context.Context) (string, error) {
	resp, err := t.c.Version(ctx)
	if err != nil {
		return "", err
	}
	if len(resp.GetMessages()) == 0 {
		return "", errors.New("empty version response")
	}
	return resp.GetMessages()[0].GetVersion().GetTag(), nil
}

// Disks lists whole disks from the node's block subsystem. It is only used to
// help the operator choose --install-disk; the installer never picks one.
func (t *talosAPI) Disks(ctx context.Context) ([]DiskInfo, error) {
	kind := resource.NewMetadata(block.NamespaceName, block.DiskType, "", resource.VersionUndefined)
	list, err := t.c.COSI.List(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("listing disks: %w", err)
	}
	out := make([]DiskInfo, 0, len(list.Items))
	for _, item := range list.Items {
		disk, ok := item.(*block.Disk)
		if !ok {
			continue
		}
		spec := disk.TypedSpec()
		out = append(out, DiskInfo{
			Device:     spec.DevPath,
			Size:       spec.Size,
			Model:      spec.Model,
			Serial:     spec.Serial,
			Transport:  spec.Transport,
			Rotational: spec.Rotational,
			Readonly:   spec.Readonly,
			CDROM:      spec.CDROM,
		})
	}
	return out, nil
}

// Interface returns the network link that carries the node IP. The installer
// template must name the real link: hardcoding eth0 works on the VM but fails
// on hardware that uses predictable names such as ens3, leaving etcd unable to
// build its advertised peer URL. Falls back to the first physical link that is
// up when the address resource is not available yet.
func (t *talosAPI) Interface(ctx context.Context, ip string) (string, error) {
	addrKind := resource.NewMetadata(network.NamespaceName, network.AddressStatusType, "", resource.VersionUndefined)
	addresses, err := t.c.COSI.List(ctx, addrKind)
	if err == nil {
		for _, item := range addresses.Items {
			addr, ok := item.(*network.AddressStatus)
			if !ok {
				continue
			}
			spec := addr.TypedSpec()
			if spec.LinkName != "" && spec.Address.Addr().String() == ip {
				return spec.LinkName, nil
			}
		}
	}

	linkKind := resource.NewMetadata(network.NamespaceName, network.LinkStatusType, "", resource.VersionUndefined)
	links, err := t.c.COSI.List(ctx, linkKind)
	if err != nil {
		return "", fmt.Errorf("listing network links: %w", err)
	}
	var names []string
	for _, item := range links.Items {
		link, ok := item.(*network.LinkStatus)
		if !ok {
			continue
		}
		spec := link.TypedSpec()
		if spec.Physical() && spec.LinkState {
			names = append(names, link.Metadata().ID())
		}
	}
	if len(names) > 0 {
		sort.Strings(names)
		return names[0], nil
	}
	return "", fmt.Errorf("no physical network link carries %s", ip)
}

func (t *talosAPI) Services(ctx context.Context) (map[string]string, error) {
	resp, err := t.c.ServiceList(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, msg := range resp.GetMessages() {
		for _, svc := range msg.GetServices() {
			out[svc.GetId()] = svc.GetState()
		}
	}
	return out, nil
}

func (t *talosAPI) Close() error { return t.c.Close() }

// rpcTimeout bounds one Talos RPC. The wait loops below use their own overall
// deadline, but without this a half-open connection would block one attempt
// forever and the overall deadline would never be checked.
const rpcTimeout = 10 * time.Second

func rpcContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, rpcTimeout)
}

// WaitForAPI polls Version until the node answers or the timeout elapses.
func WaitForAPI(ctx context.Context, api API, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		callCtx, cancel := rpcContext(ctx)
		_, err := api.Version(callCtx)
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Talos API did not answer within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// Bootstrap bootstraps etcd, treating an already-bootstrapped cluster as
// success (mirrors scripts/deploy-vm.sh).
func Bootstrap(ctx context.Context, api API) error {
	if err := api.Bootstrap(ctx); err != nil {
		if IsAlreadyBootstrapped(err) {
			return nil
		}
		return err
	}
	return nil
}

// WaitForServices polls the node until every named service reports Running (or
// the timeout elapses). Used after bootstrap to wait for etcd + kubelet.
func WaitForServices(ctx context.Context, api API, timeout time.Duration, want ...string) error {
	deadline := time.Now().Add(timeout)
	for {
		callCtx, cancel := rpcContext(ctx)
		svcs, err := api.Services(callCtx)
		cancel()
		if err == nil {
			allReady := true
			for _, name := range want {
				if !strings.Contains(svcs[name], "Running") {
					allReady = false
					break
				}
			}
			if allReady {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("services %v did not become ready within %s", want, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// IsAlreadyBootstrapped reports whether err means the cluster was bootstrapped
// on an earlier run.
func IsAlreadyBootstrapped(err error) bool {
	if err == nil {
		return false
	}
	switch client.StatusCode(err) {
	case codes.AlreadyExists, codes.FailedPrecondition:
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already bootstrapped") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "etcd is already") ||
		strings.Contains(msg, "bootstrap is already")
}
