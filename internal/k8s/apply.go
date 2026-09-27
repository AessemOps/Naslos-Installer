package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

// Apply server-side applies every Kubernetes object in a multi-document YAML
// manifest. Objects are applied in document order, so a manifest that creates a
// Namespace before the resources in it works as written. Cluster-scoped objects
// ignore the namespace; namespaced objects without one land in "default".
func (c *Client) Apply(ctx context.Context, manifest []byte, fieldManager string) error {
	if fieldManager == "" {
		fieldManager = FieldManager
	}
	objs, err := Decode(manifest)
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return errors.New("manifest contains no Kubernetes objects")
	}
	for _, u := range objs {
		if err := c.applyOne(ctx, u, fieldManager); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyOne(ctx context.Context, u *unstructured.Unstructured, fieldManager string) error {
	gvk := u.GroupVersionKind()
	if u.GetName() == "" {
		return fmt.Errorf("manifest object %s has no metadata.name", gvk.Kind)
	}
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", gvk, err)
	}

	var ri dynamic.ResourceInterface = c.dyn.Resource(mapping.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := u.GetNamespace()
		if ns == "" {
			ns = "default"
			u.SetNamespace(ns)
		}
		ri = c.dyn.Resource(mapping.Resource).Namespace(ns)
	} else {
		u.SetNamespace("")
	}

	if _, err := ri.Apply(ctx, u.GetName(), u, metav1.ApplyOptions{
		FieldManager: fieldManager,
		Force:        true,
	}); err != nil {
		return fmt.Errorf("applying %s %s: %w", gvk.Kind, u.GetName(), err)
	}
	return nil
}

// Decode splits a multi-document YAML manifest into unstructured objects. Empty
// documents (leading/trailing `---`, comments only) are skipped.
func Decode(manifest []byte) ([]*unstructured.Unstructured, error) {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	var out []*unstructured.Unstructured
	for {
		var raw map[string]interface{}
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decoding manifest: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		u := &unstructured.Unstructured{Object: raw}
		if u.GetAPIVersion() == "" || u.GetKind() == "" {
			continue
		}
		out = append(out, u)
	}
}

// LabelNamespace merges labels onto a namespace. Used to relax PodSecurity on
// the local-path namespace, whose helper pod needs hostPath volumes.
func (c *Client) LabelNamespace(ctx context.Context, name string, labels map[string]string) error {
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{"labels": labels},
	})
	if err != nil {
		return err
	}
	if _, err := c.core.CoreV1().Namespaces().Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("labelling namespace %s: %w", name, err)
	}
	return nil
}

// SetDefaultStorageClass annotates a StorageClass as the cluster default so
// chart PVCs (OpenLDAP, Prometheus, …) bind without an explicit class.
func (c *Client) SetDefaultStorageClass(ctx context.Context, name string) error {
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
	})
	if err != nil {
		return err
	}
	if _, err := c.core.StorageV1().StorageClasses().Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("setting storage class %s as default: %w", name, err)
	}
	return nil
}
