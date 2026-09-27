package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// testManifest mirrors the shape of the pack's local-path manifest: a
// cluster-scoped Namespace followed by a namespaced object.
const testManifest = `
---
# a comment-only document
---
apiVersion: v1
kind: Namespace
metadata:
  name: local-path-storage
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-path-config
  namespace: local-path-storage
data:
  key: value
---
`

func testMapper(t *testing.T) meta.RESTMapper {
	t.Helper()
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	return m
}

// applyRecord captures one server-side apply call.
type applyRecord struct {
	gvr  schema.GroupVersionResource
	ns   string
	name string
	obj  *unstructured.Unstructured
	opts metav1.ApplyOptions
}

// recordingDynamic is a minimal dynamic.Interface that records Apply calls. The
// client-go fake dynamic tracker cannot server-side apply unstructured objects
// (its strategic-merge path rejects Unstructured), so tests observe the calls
// directly instead.
type recordingDynamic struct {
	dynamic.Interface
	records []applyRecord
}

func (r *recordingDynamic) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &recordingResource{dyn: r, gvr: gvr}
}

type recordingResource struct {
	dynamic.NamespaceableResourceInterface
	dyn       *recordingDynamic
	gvr       schema.GroupVersionResource
	namespace string
}

func (r *recordingResource) Namespace(ns string) dynamic.ResourceInterface {
	return &recordingResource{dyn: r.dyn, gvr: r.gvr, namespace: ns}
}

func (r *recordingResource) Apply(_ context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, _ ...string) (*unstructured.Unstructured, error) {
	r.dyn.records = append(r.dyn.records, applyRecord{
		gvr: r.gvr, ns: r.namespace, name: name, obj: obj.DeepCopy(), opts: opts,
	})
	return obj, nil
}

func testClient(t *testing.T, objs ...runtime.Object) (*Client, *recordingDynamic, kubernetes.Interface) {
	t.Helper()
	dyn := &recordingDynamic{}
	core := k8sfake.NewSimpleClientset(objs...)
	return NewWith(dyn, core, testMapper(t)), dyn, core
}

func TestDecode(t *testing.T) {
	objs, err := Decode([]byte(testManifest))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("got %d objects, want 2", len(objs))
	}
	if objs[0].GetKind() != "Namespace" || objs[1].GetKind() != "ConfigMap" {
		t.Fatalf("unexpected kinds: %s, %s", objs[0].GetKind(), objs[1].GetKind())
	}
}

func TestDecodeEmpty(t *testing.T) {
	objs, err := Decode([]byte("# nothing\n---\n"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(objs) != 0 {
		t.Fatalf("got %d objects, want 0", len(objs))
	}
}

func TestApply(t *testing.T) {
	c, dyn, _ := testClient(t)
	if err := c.Apply(context.Background(), []byte(testManifest), "test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(dyn.records) != 2 {
		t.Fatalf("got %d apply calls, want 2", len(dyn.records))
	}

	ns := dyn.records[0]
	if ns.gvr.Resource != "namespaces" || ns.ns != "" || ns.name != "local-path-storage" {
		t.Fatalf("unexpected namespace apply: %+v", ns)
	}
	cm := dyn.records[1]
	if cm.gvr.Resource != "configmaps" || cm.ns != "local-path-storage" || cm.name != "local-path-config" {
		t.Fatalf("unexpected configmap apply: %+v", cm)
	}
	if cm.opts.FieldManager != "test" || !cm.opts.Force {
		t.Fatalf("unexpected apply options: %+v", cm.opts)
	}
	if got, _, _ := unstructured.NestedString(cm.obj.Object, "data", "key"); got != "value" {
		t.Fatalf("configmap data lost: %q", got)
	}
}

func TestApplyDefaultFieldManager(t *testing.T) {
	c, dyn, _ := testClient(t)
	if err := c.Apply(context.Background(), []byte(testManifest), ""); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if dyn.records[0].opts.FieldManager != FieldManager {
		t.Fatalf("field manager = %q, want %q", dyn.records[0].opts.FieldManager, FieldManager)
	}
}

func TestApplyRejectsEmptyManifest(t *testing.T) {
	c, _, _ := testClient(t)
	if err := c.Apply(context.Background(), []byte("# nothing\n"), "test"); err == nil {
		t.Fatal("expected an error for a manifest with no objects")
	}
}

func TestApplyDefaultsNamespace(t *testing.T) {
	c, dyn, _ := testClient(t)
	manifest := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: orphan\n")
	if err := c.Apply(context.Background(), manifest, "test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(dyn.records) != 1 || dyn.records[0].ns != "default" {
		t.Fatalf("namespaced object did not default to 'default': %+v", dyn.records)
	}
}

func TestApplyRejectsUnmappedKind(t *testing.T) {
	c, _, _ := testClient(t)
	manifest := []byte("apiVersion: example.com/v1\nkind: Widget\nmetadata:\n  name: w\n")
	err := c.Apply(context.Background(), manifest, "test")
	if err == nil || !strings.Contains(err.Error(), "resolving") {
		t.Fatalf("expected a resolution error, got %v", err)
	}
}

func TestLabelNamespace(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "local-path-storage",
		Labels: map[string]string{"keep": "me"},
	}}
	c, _, core := testClient(t, ns)
	err := c.LabelNamespace(context.Background(), "local-path-storage", map[string]string{
		"pod-security.kubernetes.io/enforce": "privileged",
	})
	if err != nil {
		t.Fatalf("LabelNamespace: %v", err)
	}
	got, err := core.CoreV1().Namespaces().Get(context.Background(), "local-path-storage", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels["pod-security.kubernetes.io/enforce"] != "privileged" {
		t.Fatalf("psa label not set: %v", got.Labels)
	}
	if got.Labels["keep"] != "me" {
		t.Fatalf("merge dropped the pre-existing label: %v", got.Labels)
	}
}

func TestSetDefaultStorageClass(t *testing.T) {
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}}
	c, _, core := testClient(t, sc)
	if err := c.SetDefaultStorageClass(context.Background(), "local-path"); err != nil {
		t.Fatalf("SetDefaultStorageClass: %v", err)
	}
	got, err := core.StorageV1().StorageClasses().Get(context.Background(), "local-path", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Annotations["storageclass.kubernetes.io/is-default-class"] != "true" {
		t.Fatalf("default annotation not set: %v", got.Annotations)
	}
}

func TestWaitForDeploymentReady(t *testing.T) {
	d := readyDeployment("local-path-storage", "local-path-provisioner", 1, 1)
	c, _, _ := testClient(t, d)
	if err := c.WaitForDeployment(context.Background(), "local-path-storage", "local-path-provisioner", time.Second); err != nil {
		t.Fatalf("WaitForDeployment: %v", err)
	}
}

func TestWaitForDeploymentTimeout(t *testing.T) {
	d := readyDeployment("local-path-storage", "local-path-provisioner", 1, 0)
	c, _, _ := testClient(t, d)
	err := c.WaitForDeployment(context.Background(), "local-path-storage", "local-path-provisioner", time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
}

func TestWaitForDaemonSetReady(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "cilium", Generation: 1},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     1,
			DesiredNumberScheduled: 1,
			NumberReady:            1,
		},
	}
	c, _, _ := testClient(t, ds)
	if err := c.WaitForDaemonSet(context.Background(), "kube-system", "cilium", time.Second); err != nil {
		t.Fatalf("WaitForDaemonSet: %v", err)
	}
}

func readyDeployment(ns, name string, desired, ready int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Generation: 1},
		Spec:       appsv1.DeploymentSpec{Replicas: &desired},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           ready,
			ReadyReplicas:      ready,
			UpdatedReplicas:    ready,
		},
	}
}
