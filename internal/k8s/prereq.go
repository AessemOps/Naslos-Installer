package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureNamespace creates a namespace or, if it already exists, merges the
// labels onto it. The engine pre-creates `naslos` so the naslos-talosconfig
// Secret can exist before the API pod starts.
func (c *Client) EnsureNamespace(ctx context.Context, name string, labels map[string]string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
	if _, err := c.core.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating namespace %s: %w", name, err)
		}
		return c.LabelNamespace(ctx, name, labels)
	}
	return nil
}

// UpsertSecret creates or replaces an opaque Secret's data.
func (c *Client) UpsertSecret(ctx context.Context, namespace, name string, data map[string][]byte, labels map[string]string) error {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		Data:       data,
	}
	if _, err := c.core.CoreV1().Secrets(namespace).Create(ctx, s, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating secret %s/%s: %w", namespace, name, err)
		}
		if _, err := c.core.CoreV1().Secrets(namespace).Update(ctx, s, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating secret %s/%s: %w", namespace, name, err)
		}
	}
	return nil
}
