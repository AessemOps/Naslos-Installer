package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// Exec runs a command in a pod container and returns stdout and stderr. It uses
// the SPDY executor on the client's REST config; a Client built with NewWith
// (tests) has none and returns an error.
func (c *Client) Exec(ctx context.Context, namespace, pod, container string, command []string) (string, string, error) {
	return c.ExecWithStdin(ctx, namespace, pod, container, command, "")
}

// ExecWithStdin is Exec with a body on stdin. Secrets must travel here rather
// than in argv: a pod exec request URI (which contains the command) is recorded
// by the kube-apiserver audit log at the default Metadata level.
func (c *Client) ExecWithStdin(ctx context.Context, namespace, pod, container string, command []string, stdin string) (string, string, error) {
	if c.rest == nil {
		return "", "", fmt.Errorf("exec is not available on this client")
	}
	req := c.core.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     stdin != "",
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.rest, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("creating the exec stream: %w", err)
	}
	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, streamOptions(stdin, &stdout, &stderr))
	return stdout.String(), stderr.String(), err
}

// streamOptions must leave Stdin as a nil interface when there is no stdin.
// Assigning a nil *strings.Reader produced a non-nil io.Reader, so client-go
// opened a stdin stream that the kubelet had been told not to expect
// (PodExecOptions.Stdin=false); stream setup then failed with "Timeout
// occurred" even though the command itself ran (the TOTP device was created,
// but its output never reached the engine).
func streamOptions(stdin string, stdout, stderr io.Writer) remotecommand.StreamOptions {
	opts := remotecommand.StreamOptions{Stdout: stdout, Stderr: stderr}
	if stdin != "" {
		opts.Stdin = strings.NewReader(stdin)
	}
	return opts
}

// SecretValue decodes one key of a Secret.
func (c *Client) SecretValue(ctx context.Context, namespace, name, key string) (string, error) {
	s, err := c.core.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("reading secret %s/%s: %w", namespace, name, err)
	}
	v, ok := s.Data[key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s has no key %q", namespace, name, key)
	}
	return string(v), nil
}

// PodForDeployment returns the name of a Running pod owned by a Deployment, so
// the engine can exec into a workload addressed by its stable Deployment name.
func (c *Client) PodForDeployment(ctx context.Context, namespace, deployment string) (string, error) {
	d, err := c.core.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("reading deployment %s/%s: %w", namespace, deployment, err)
	}
	sel, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("deployment %s/%s has an invalid selector: %w", namespace, deployment, err)
	}
	pods, err := c.core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return "", fmt.Errorf("listing pods for %s/%s: %w", namespace, deployment, err)
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("no Running pod for deployment %s/%s", namespace, deployment)
}
