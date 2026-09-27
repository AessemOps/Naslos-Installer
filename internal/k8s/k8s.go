// Package k8s applies the cluster-side install steps over the kubeconfig the
// engine fetches from the freshly-bootstrapped node.
//
// It owns the parts of the install that talk to the Kubernetes API rather than
// the Talos API: applying the pinned local-path manifest, relaxing PodSecurity
// on its namespace, making local-path the default StorageClass, and waiting for
// workloads (the CNI DaemonSet, the provisioner Deployment) to become ready.
//
// All manifests come from the verified install pack; the engine never shells out
// to kubectl (SEC-1). Objects are applied server-side so a re-run of an
// interrupted install converges instead of colliding.
package k8s

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// FieldManager identifies the engine as the owner of the fields it applies.
const FieldManager = "naslos-install"

// Client is a Kubernetes client with a discovery-backed REST mapper so the same
// code applies built-in and CRD resources.
type Client struct {
	dyn    dynamic.Interface
	core   kubernetes.Interface
	mapper meta.RESTMapper
}

// NewFromKubeconfig parses the fetched kubeconfig and builds a Client.
func NewFromKubeconfig(kubeconfig []byte) (*Client, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}
	return New(cfg)
}

// New builds a Client from a REST config.
func New(cfg *rest.Config) (*Client, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating the dynamic client: %w", err)
	}
	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating the typed client: %w", err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating the discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))
	return &Client{dyn: dyn, core: core, mapper: mapper}, nil
}

// NewWith builds a Client from explicit interfaces. Tests use it with the fake
// clients; production code uses New / NewFromKubeconfig.
func NewWith(dyn dynamic.Interface, core kubernetes.Interface, mapper meta.RESTMapper) *Client {
	return &Client{dyn: dyn, core: core, mapper: mapper}
}

// WaitForDaemonSet polls a DaemonSet until every desired pod is ready.
func (c *Client) WaitForDaemonSet(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return poll(ctx, timeout, fmt.Sprintf("DaemonSet %s/%s", namespace, name), func() (bool, error) {
		ds, err := c.core.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		s := ds.Status
		return s.ObservedGeneration >= ds.Generation &&
			s.DesiredNumberScheduled > 0 &&
			s.NumberReady >= s.DesiredNumberScheduled, nil
	})
}

// WaitForDeployment polls a Deployment until all its replicas are ready.
func (c *Client) WaitForDeployment(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return poll(ctx, timeout, fmt.Sprintf("Deployment %s/%s", namespace, name), func() (bool, error) {
		d, err := c.core.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		s := d.Status
		return s.ObservedGeneration >= d.Generation &&
			s.ReadyReplicas >= desired &&
			s.UpdatedReplicas >= desired, nil
	})
}

// poll calls check until it reports ready or the timeout elapses. A check error
// is surfaced only when the timeout is reached, so transient API errors (the
// resource not existing yet, a TLS blip) do not abort the wait.
func poll(ctx context.Context, timeout time.Duration, what string, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		ready, err := check()
		if err == nil && ready {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("%s did not become ready within %s: %w", what, timeout, lastErr)
			}
			return fmt.Errorf("%s did not become ready within %s", what, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
