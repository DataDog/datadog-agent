// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedynamic "k8s.io/client-go/dynamic/fake"
)

// Test partitions for the controller suite:
// - entity kind: pod | node
// - value kind: bool with source comparison | bool field-derived | string set with fallback
// - selector: matching | non-matching (labels) | non-matching (namespace)
// - lifecycle: initial write | flip after hold | rule deletion cleanup | user drift restored
// - string set: in-set value | out-of-set value | missing labels (onError default)

var (
	leaseGVR     = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}
	namespaceGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
)

// fakeClock gives the controller a controllable time for flip-hold tests.
type fakeClock struct {
	mu     sync.Mutex
	base   time.Time
	offset time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base.Add(c.offset)
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset += d
}

func newFakeClock() *fakeClock {
	return &fakeClock{base: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func tagRuleObject(t *testing.T, name string, spec TagRuleSpec) *unstructured.Unstructured {
	t.Helper()
	rule := TagRule{
		TypeMeta:   metav1.TypeMeta{APIVersion: GroupName + "/" + GroupVersion, Kind: TagRuleKind},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       spec,
	}
	obj, err := rule.toUnstructured()
	if err != nil {
		t.Fatalf("toUnstructured: %v", err)
	}
	return obj
}

func leaseObject(namespace, name, holder string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "coordination.k8s.io/v1",
		"kind":       "Lease",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       map[string]any{"holderIdentity": holder},
	}}
}

func namespaceObject(name string, labels map[string]string) *unstructured.Unstructured {
	ns := newEntity(EntityPod, name, "", labels) // kind/namespace fields overridden below
	ns.Object["kind"] = "Namespace"
	return ns
}

func newFakeClient(objects ...*unstructured.Unstructured) *fakedynamic.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		tagRuleGVR:   "TagRuleList",
		podGVR:       "PodList",
		nodeGVR:      "NodeList",
		namespaceGVR: "NamespaceList",
		leaseGVR:     "LeaseList",
	}
	return fakedynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, asRuntimeObjects(objects...)...)
}

func asRuntimeObjects(objects ...*unstructured.Unstructured) []runtime.Object {
	out := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		out = append(out, runtime.Object(obj))
	}
	return out
}

func startController(t *testing.T, client *fakedynamic.FakeDynamicClient, clock *fakeClock, opts ...Option) (*Controller, context.CancelFunc) {
	t.Helper()
	controller, err := NewController(client, append([]Option{WithNow(clock.Now), WithIntervals(25*time.Millisecond, 25*time.Millisecond)}, opts...)...)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = controller.Run(ctx) }()
	return controller, cancel
}

func eventually(t *testing.T, message string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within timeout: %s", message)
}

func getEntity(t *testing.T, client *fakedynamic.FakeDynamicClient, gvr schema.GroupVersionResource, namespace, name string) *unstructured.Unstructured {
	t.Helper()
	var (
		obj *unstructured.Unstructured
		err error
	)
	if namespace != "" {
		obj, err = client.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	} else {
		obj, err = client.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
	}
	if err != nil {
		return nil
	}
	return obj
}

func podTags(t *testing.T, obj *unstructured.Unstructured, container string) map[string]string {
	t.Helper()
	if obj == nil {
		return nil
	}
	raw := obj.GetAnnotations()[fmt.Sprintf(ADContainerTagsAnnotationFormat, container)]
	if raw == "" {
		return nil
	}
	var tags map[string]string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		t.Fatalf("parsing %s annotation: %v", ADContainerTagsAnnotationFormat, err)
	}
	return tags
}

func int64Ptr(v int64) *int64 { return &v }

// TestControllerIsLeaderFlow covers: pod entity; matching and non-matching selectors
// (labels and namespace); bool with Lease source comparison; per-container annotation
// writes; user key preservation; flip after the hold; deletion cleanup via finalizer.
func TestControllerIsLeaderFlow(t *testing.T) {
	clock := newFakeClock()
	client := newFakeClient(
		tagRuleObject(t, "is-leader-my-controller", TagRuleSpec{
			Entity: EntityPod,
			Selector: Selector{
				Namespace:   "my-namespace",
				MatchLabels: map[string]string{"app": "my-controller"},
			},
			Tag: "is_leader",
			Source: &SourceRef{
				Kind:      "Lease",
				Namespace: "entity.metadata.namespace",
				Name:      "entity.metadata.labels['app'] + '-leader-election'",
			},
			Value:           Value{Type: ValueBool, Bool: &BoolValue{Expression: "source.spec.holderIdentity == entity.metadata.name"}},
			FlipHoldSeconds: int64Ptr(15),
		}),
		// Ben matches and holds the lease; his app container already carries a user tag.
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", map[string]string{"app": "my-controller"})
			setContainers(pod, "app", "istio-proxy")
			setAnnotations(pod, map[string]string{adTagsAnnotationKey("app"): `{"team":"core"}`})
			return pod
		}(),
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "eva-lu-ator", "my-namespace", map[string]string{"app": "my-controller"})
			setContainers(pod, "app")
			return pod
		}(),
		// Louis matches the namespace but not the labels.
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "louis-reasoner", "my-namespace", map[string]string{"app": "other"})
			setContainers(pod, "app")
			return pod
		}(),
		// Alyssa matches the labels but not the namespace.
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "alyssa-hacker", "other-namespace", map[string]string{"app": "my-controller"})
			setContainers(pod, "app")
			return pod
		}(),
		leaseObject("my-namespace", "my-controller-leader-election", "ben-bitdiddle"),
	)
	_, _ = startController(t, client, clock)

	// Initial write: Ben is leader on both containers; his user tag survives.
	// Eva matches the rule and carries the negative value.
	eventually(t, "ben tagged leader with user key preserved", func() bool {
		ben := getEntity(t, client, podGVR, "my-namespace", "ben-bitdiddle")
		appTags := podTags(t, ben, "app")
		proxyTags := podTags(t, ben, "istio-proxy")
		return appTags["team"] == "core" && appTags["is_leader"] == "true" && proxyTags["is_leader"] == "true"
	})
	eventually(t, "eva tagged not leader", func() bool {
		return podTags(t, getEntity(t, client, podGVR, "my-namespace", "eva-lu-ator"), "app")["is_leader"] == "false"
	})
	eventually(t, "ben ledger written", func() bool {
		return getEntity(t, client, podGVR, "my-namespace", "ben-bitdiddle").GetAnnotations()[ManagedTagKeysAnnotation] == "is_leader"
	})

	// Non-matching pods are untouched.
	time.Sleep(200 * time.Millisecond)
	for pod, ns := range map[string]string{"louis-reasoner": "my-namespace", "alyssa-hacker": "other-namespace"} {
		if ann := getEntity(t, client, podGVR, ns, pod).GetAnnotations(); len(ann) != 0 {
			t.Errorf("non-matching pod %s has annotations: %v", pod, ann)
		}
	}

	// Flip: the lease moves to Eva after the flip hold.
	clock.Advance(16 * time.Second)
	lease := leaseObject("my-namespace", "my-controller-leader-election", "eva-lu-ator")
	if _, err := client.Resource(leaseGVR).Namespace("my-namespace").Update(context.Background(), lease, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("updating lease: %v", err)
	}
	eventually(t, "leadership flipped to eva", func() bool {
		return podTags(t, getEntity(t, client, podGVR, "my-namespace", "eva-lu-ator"), "app")["is_leader"] == "true" &&
			podTags(t, getEntity(t, client, podGVR, "my-namespace", "ben-bitdiddle"), "app")["is_leader"] == "false"
	})

	// Deletion: the finalizer holds the CR until every matched pod is swept;
	// Ben's user key outlives the controller's owned key.
	if err := client.Resource(tagRuleGVR).Delete(context.Background(), "is-leader-my-controller", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("deleting rule: %v", err)
	}
	eventually(t, "owned tags swept after rule deletion", func() bool {
		ben := getEntity(t, client, podGVR, "my-namespace", "ben-bitdiddle")
		appTags := podTags(t, ben, "app")
		ann := ben.GetAnnotations()
		return appTags["is_leader"] == "" && appTags["team"] == "core" &&
			podTags(t, ben, "istio-proxy") == nil &&
			ann[ManagedTagKeysAnnotation] == "" &&
			podTags(t, getEntity(t, client, podGVR, "my-namespace", "eva-lu-ator"), "app") == nil
	})
	eventually(t, "CR deleted after finalizer released", func() bool {
		return getEntity(t, client, tagRuleGVR, "", "is-leader-my-controller") == nil
	})
}

// TestControllerOwningTeamStringSet covers: string set with pod label, namespace
// fallback, out-of-set label (onError default) and missing labels everywhere.
func TestControllerOwningTeamStringSet(t *testing.T) {
	clock := newFakeClock()
	client := newFakeClient(
		tagRuleObject(t, "owning-team-pods", TagRuleSpec{
			Entity: EntityPod,
			Tag:    "owning_team",
			Source: &SourceRef{Kind: "Namespace", Name: "entity.metadata.namespace"},
			Value: Value{Type: ValueStringSet, StringSet: &StringSetValue{
				Values:     []string{"frontend", "backend", "data", "infra", "unknown"},
				Expression: `('owner' in entity.metadata.labels) ? entity.metadata.labels['owner'] : source.metadata.labels['owner']`,
				OnError:    OnErrorDefault,
				Default:    "unknown",
			}},
		}),
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", map[string]string{"owner": "frontend"})
			setContainers(pod, "app")
			return pod
		}(),
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "louis-reasoner", "my-namespace", nil)
			setContainers(pod, "app")
			return pod
		}(),
		func() *unstructured.Unstructured {
			// mobile is outside the declared set: the default is written.
			pod := newEntity(EntityPod, "eva-lu-ator", "my-namespace", map[string]string{"owner": "mobile"})
			setContainers(pod, "app")
			return pod
		}(),
		func() *unstructured.Unstructured {
			// Namespace without an owner label: expression errors, default wins.
			pod := newEntity(EntityPod, "alyssa-hacker", "blank-namespace", nil)
			setContainers(pod, "app")
			return pod
		}(),
		namespaceObject("my-namespace", map[string]string{"owner": "infra"}),
		namespaceObject("blank-namespace", nil),
	)
	controller, _ := startController(t, client, clock)

	for _, entry := range []struct {
		pod, namespace, want string
	}{
		{"ben-bitdiddle", "my-namespace", "frontend"}, // pod label wins
		{"louis-reasoner", "my-namespace", "infra"},   // namespace fallback
		{"eva-lu-ator", "my-namespace", "unknown"},    // out-of-set value
		{"alyssa-hacker", "blank-namespace", "unknown"},
	} {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if podTags(t, getEntity(t, client, podGVR, entry.namespace, entry.pod), "app")["owning_team"] == entry.want {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if got := podTags(t, getEntity(t, client, podGVR, entry.namespace, entry.pod), "app")["owning_team"]; got != entry.want {
			controller.informersMu.Lock()
			informers := make([]string, 0)
			for gvr := range controller.informers {
				informers = append(informers, gvr.String())
			}
			controller.informersMu.Unlock()
			t.Fatalf("owning_team for %s: got %q, want %q; diagnostics queue=%d rules=%d informers=%v",
				entry.pod, got, entry.want, controller.queue.Len(), len(controller.rules), informers)
		}
	}
}

// TestControllerNodeSchedulable covers: node entity; field-derived bool; prefix
// annotations; flip after the hold when a node is uncordoned.
func TestControllerNodeSchedulable(t *testing.T) {
	clock := newFakeClock()
	nodeB := newEntity(EntityNode, "node-b", "", nil)
	nodeB.Object["spec"].(map[string]any)["unschedulable"] = true

	client := newFakeClient(
		tagRuleObject(t, "is-schedulable-nodes", TagRuleSpec{
			Entity: EntityNode,
			Tag:    "is_schedulable",
			Value: Value{Type: ValueBool, Bool: &BoolValue{
				Expression: "!has(entity.spec.unschedulable) || !entity.spec.unschedulable",
			}},
			FlipHoldSeconds: int64Ptr(15),
		}),
		newEntity(EntityNode, "node-a", "", nil),
		nodeB,
	)
	_, _ = startController(t, client, clock)

	eventually(t, "node-a schedulable", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-a").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "true"
	})
	eventually(t, "node-b cordoned", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-b").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "false"
	})

	// Uncordon node-b: the flip is held first, then written after the hold.
	nodeB = getEntity(t, client, nodeGVR, "", "node-b")
	nodeB.Object["spec"].(map[string]any)["unschedulable"] = false
	if _, err := client.Resource(nodeGVR).Update(context.Background(), nodeB, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("uncordoning node-b: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := getEntity(t, client, nodeGVR, "", "node-b").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"]; got != "false" {
		t.Errorf("flip inside hold: got %q, want false", got)
	}

	clock.Advance(16 * time.Second)
	eventually(t, "node-b schedulable after hold", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-b").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "true"
	})
}

// TestControllerDriftRestored covers: a direct user edit of a controller-owned
// key is restored on the next resync (controller wins).
func TestControllerDriftRestored(t *testing.T) {
	clock := newFakeClock()
	client := newFakeClient(
		tagRuleObject(t, "is-schedulable-nodes", TagRuleSpec{
			Entity: EntityNode,
			Tag:    "is_schedulable",
			Value:  Value{Type: ValueBool, Bool: &BoolValue{Expression: "true"}},
		}),
		newEntity(EntityNode, "node-a", "", nil),
	)
	_, _ = startController(t, client, clock)
	eventually(t, "node-a tagged", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-a").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "true"
	})

	node := getEntity(t, client, nodeGVR, "", "node-a")
	annotations := node.GetAnnotations()
	annotations[NodeTagAnnotationPrefix+"is_schedulable"] = "false"
	node.SetAnnotations(annotations)
	if _, err := client.Resource(nodeGVR).Update(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("drift edit: %v", err)
	}

	eventually(t, "drift restored", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-a").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "true"
	})
}

// TestControllerTagKeyConflict covers: a second rule claiming an owned tag key
// is rejected with an error status and never writes.
func TestControllerTagKeyConflict(t *testing.T) {
	clock := newFakeClock()
	client := newFakeClient(
		tagRuleObject(t, "rule-one", TagRuleSpec{
			Entity: EntityNode,
			Tag:    "is_schedulable",
			Value:  Value{Type: ValueBool, Bool: &BoolValue{Expression: "true"}},
		}),
		tagRuleObject(t, "rule-two", TagRuleSpec{
			Entity: EntityNode,
			Tag:    "is_schedulable", // duplicate tag key
			Value:  Value{Type: ValueBool, Bool: &BoolValue{Expression: "false"}},
		}),
		newEntity(EntityNode, "node-a", "", nil),
	)
	_, _ = startController(t, client, clock)

	eventually(t, "rule-two reports error status", func() bool {
		rule := getEntity(t, client, tagRuleGVR, "", "rule-two")
		if rule == nil {
			return false
		}
		status, _, _ := unstructured.NestedString(rule.Object, "status", "state")
		return status == string(RuleStateError)
	})

	// The first rule still owns the key and writes.
	eventually(t, "rule-one writes", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-a").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "true"
	})
}

// TestControllerStatusReports covers: matched/tagged counts and the string-set
// value histogram for an active rule.
func TestControllerStatusReports(t *testing.T) {
	clock := newFakeClock()
	client := newFakeClient(
		tagRuleObject(t, "owning-team-pods", TagRuleSpec{
			Entity: EntityPod,
			Tag:    "owning_team",
			Value: Value{Type: ValueStringSet, StringSet: &StringSetValue{
				Values:     []string{"frontend", "backend"},
				Expression: "entity.metadata.labels['owner']",
				OnError:    OnErrorDefault,
				Default:    "frontend",
			}},
		}),
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", map[string]string{"owner": "frontend"})
			setContainers(pod, "app")
			return pod
		}(),
		func() *unstructured.Unstructured {
			pod := newEntity(EntityPod, "eva-lu-ator", "my-namespace", map[string]string{"owner": "backend"})
			setContainers(pod, "app")
			return pod
		}(),
	)
	_, _ = startController(t, client, clock)

	eventually(t, "status reports matched and tagged counts with histogram", func() bool {
		rule := getEntity(t, client, tagRuleGVR, "", "owning-team-pods")
		if rule == nil {
			return false
		}
		matched, _, _ := unstructured.NestedInt64(rule.Object, "status", "entities", "matched")
		tagged, _, _ := unstructured.NestedInt64(rule.Object, "status", "entities", "tagged")
		frontend, _, _ := unstructured.NestedInt64(rule.Object, "status", "values", "observed", "frontend")
		backend, _, _ := unstructured.NestedInt64(rule.Object, "status", "values", "observed", "backend")
		state, _, _ := unstructured.NestedString(rule.Object, "status", "state")
		return matched == 2 && tagged == 2 && frontend == 1 && backend == 1 && state == "active"
	})
}

// TestControllerLeaderGate covers: no reconciles while not the leader; work
// resumes when leadership is gained (including rules cached from the store by
// the resync loop, since a process that started as follower has an empty rule
// cache).
func TestControllerLeaderGate(t *testing.T) {
	clock := newFakeClock()
	client := newFakeClient(
		tagRuleObject(t, "is-schedulable-nodes", TagRuleSpec{
			Entity: EntityNode,
			Tag:    "is_schedulable",
			Value:  Value{Type: ValueBool, Bool: &BoolValue{Expression: "true"}},
		}),
		newEntity(EntityNode, "node-a", "", nil),
	)

	var leadership atomic.Bool
	startController(t, client, clock, WithLeaderCheck(leadership.Load))

	// Follower phase: the controller runs but must not write anything.
	time.Sleep(300 * time.Millisecond)
	if ann := getEntity(t, client, nodeGVR, "", "node-a").GetAnnotations(); len(ann) != 0 {
		t.Fatalf("follower wrote annotations: %v", ann)
	}

	// Leadership gained: the resync loop re-enqueues from the informer store
	// and the tag appears.
	leadership.Store(true)
	eventually(t, "leader writes tag", func() bool {
		return getEntity(t, client, nodeGVR, "", "node-a").GetAnnotations()[NodeTagAnnotationPrefix+"is_schedulable"] == "true"
	})
}
