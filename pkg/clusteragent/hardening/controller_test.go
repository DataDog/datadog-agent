// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func testDeployment(annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop", UID: "uid-1", Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "app", Image: "web:1.2", ReadinessProbe: &corev1.Probe{}},
				}},
			},
		},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
	}
}

// newTestController returns a controller whose fake API server, like a real
// one, persists nothing for dry-run patches.
func newTestController(t *testing.T, d *appsv1.Deployment, reqs ...[]byte) (*Controller, *fake.Clientset, *Store) {
	client := fake.NewClientset(d)
	client.PrependReactor("patch", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if len(a.(k8stesting.PatchActionImpl).PatchOptions.DryRun) > 0 {
			obj, err := client.Tracker().Get(appsv1.SchemeGroupVersion.WithResource("deployments"), "shop", "web")
			return true, obj, err
		}
		return false, nil, nil
	})
	store := NewStore(testCluster)
	setRC(t, store, reqs...)
	c := NewController(client, store, func() bool { return true })
	c.syncWait = 0
	return c, client, store
}

// testReplicaSet returns a ReplicaSet of testDeployment whose template lists ids.
func testReplicaSet(name string, age time.Duration, ids, image string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", UID: types.UID("uid-" + name), Labels: map[string]string{"app": "web"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
			OwnerReferences:   []metav1.OwnerReference{{Kind: "Deployment", Name: "web", UID: "uid-1", Controller: ptr.To(true)}},
		},
		Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{RequestsAnnotation: ids}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: image}}},
		}},
	}
}

// rollout returns the ReplicaSet the Deployment controller creates for d's
// current template, as if age ago.
func rollout(d *appsv1.Deployment, name string, age time.Duration) *appsv1.ReplicaSet {
	rs := testReplicaSet(name, age, "", "")
	rs.Spec.Template = *d.Spec.Template.DeepCopy()
	if rs.Spec.Template.Labels == nil {
		rs.Spec.Template.Labels = map[string]string{}
	}
	rs.Spec.Template.Labels[appsv1.DefaultDeploymentUniqueLabelKey] = name
	return rs
}

// hardenedPod returns a pod of rs on which the webhook applied every request rs lists.
func hardenedPod(name string, rs *appsv1.ReplicaSet, status corev1.ContainerStatus) *corev1.Pod {
	results := map[string]string{}
	for _, id := range ParseIDs(rs.Spec.Template.Annotations[RequestsAnnotation]) {
		results[id] = "applied"
	}
	raw, _ := json.Marshal(results)
	annotations := maps.Clone(rs.Spec.Template.Annotations)
	annotations[AppliedAnnotation] = string(raw)
	status.Name = "app"
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", Labels: map[string]string{"app": "web"}, Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: ptr.To(true)}},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{status}},
	}
}

func restarted(reason string) corev1.ContainerStatus {
	return corev1.ContainerStatus{RestartCount: 1, LastTerminationState: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: reason, ExitCode: 1},
	}}
}

// enforced returns a Deployment whose template lists ids, as the leader left it.
func enforced(ids string) *appsv1.Deployment {
	d := testDeployment(map[string]string{RequestsAnnotation: ids})
	d.Spec.Template.Labels = map[string]string{EnabledLabel: "true", "app": "web"}
	return d
}

func create(t *testing.T, client *fake.Clientset, objs ...runtime.Object) {
	for _, o := range objs {
		require.NoError(t, client.Tracker().Add(o))
	}
}

func setRC(t *testing.T, s *Store, reqs ...[]byte) {
	updates := map[string]state.RawConfig{}
	for i, r := range reqs {
		updates[string(rune('a'+i))] = state.RawConfig{Config: r}
	}
	s.OnRCUpdate(updates, func(_ string, st state.ApplyStatus) { require.Empty(t, st.Error) })
}

func getDeployment(t *testing.T, client *fake.Clientset) *appsv1.Deployment {
	d, err := client.AppsV1().Deployments("shop").Get(context.Background(), "web", metav1.GetOptions{})
	require.NoError(t, err)
	return d
}

func patches(client *fake.Clientset) (dryRuns, real int) {
	for _, a := range client.Actions() {
		if p, ok := a.(k8stesting.PatchActionImpl); ok {
			if len(p.PatchOptions.DryRun) > 0 {
				dryRuns++
			} else {
				real++
			}
		}
	}
	return
}

func TestControllerAppliesRequest(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	c.reconcile(context.Background())

	d := getDeployment(t, client)
	assert.Equal(t, "true", d.Spec.Template.Labels[EnabledLabel])
	assert.Equal(t, "req-1", d.Spec.Template.Annotations[RequestsAnnotation])
	assert.Nil(t, d.Spec.Template.Spec.Containers[0].SecurityContext, "the leader never writes the securityContext")
	dryRuns, real := patches(client)
	assert.Equal(t, 2, dryRuns)
	assert.Equal(t, 1, real)

	// Idempotent.
	c.reconcile(context.Background())
	_, real = patches(client)
	assert.Equal(t, 1, real)
}

func TestControllerAppendsToInertIDs(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(map[string]string{RequestsAnnotation: "old"}), rawRequest(t, nil))
	c.reconcile(context.Background())
	assert.Equal(t, "old,req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerRejects(t *testing.T) {
	tests := map[string]func(d *appsv1.Deployment){
		"created after the cluster was added": func(d *appsv1.Deployment) { d.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Hour)) },
		"privileged": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptr.To(true)}
		},
		"missing container": func(d *appsv1.Deployment) { d.Spec.Template.Spec.Containers[0].Name = "other" },
		"only minReadySeconds, no probe": func(d *appsv1.Deployment) {
			d.Spec.MinReadySeconds = 30
			d.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
		},
		"no revision history": func(d *appsv1.Deployment) { d.Spec.RevisionHistoryLimit = ptr.To(int32(0)) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			d := testDeployment(nil)
			mutate(d)
			c, client, _ := newTestController(t, d, rawRequest(t, nil))
			c.reconcile(context.Background())
			assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
			assert.Contains(t, c.done, "req-1")
		})
	}
}

func TestControllerAcceptsStartupProbe(t *testing.T) {
	d := testDeployment(nil)
	d.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
	d.Spec.Template.Spec.Containers[0].StartupProbe = &corev1.Probe{}
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerRefusesOwnDeployment(t *testing.T) {
	for name, self := range map[string]types.NamespacedName{
		"own Deployment":   {Namespace: "shop", Name: "web"},
		"unknown own name": {Namespace: "shop"},
	} {
		t.Run(name, func(t *testing.T) {
			c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
			c.self = self
			c.reconcile(context.Background())
			assert.Contains(t, c.done, "req-1")
			_, real := patches(client)
			assert.Zero(t, real)
		})
	}
}

func TestControllerWaitsForReplicasToLoadRequest(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	c.syncWait = time.Minute
	now := time.Now()
	c.now = func() time.Time { return now }

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])

	now = now.Add(time.Minute)
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerValidate(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, func(m map[string]any) { m["action"] = "validate" }))
	c.reconcile(context.Background())
	assert.Contains(t, c.done, "req-1")
	dryRuns, real := patches(client)
	assert.Equal(t, 2, dryRuns)
	assert.Zero(t, real, "validate writes nothing")

	c.reconcile(context.Background())
	dryRuns, _ = patches(client)
	assert.Equal(t, 2, dryRuns, "validated once")
}

func TestControllerValidateRejects(t *testing.T) {
	d := testDeployment(nil)
	d.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
	c, client, _ := newTestController(t, d, rawRequest(t, func(m map[string]any) { m["action"] = "validate" }))
	c.reconcile(context.Background())
	assert.Contains(t, c.done, "req-1")
	dryRuns, real := patches(client)
	assert.Zero(t, dryRuns+real, "no dry run after a failed pre-flight")
}

func TestControllerRejectsMissingDeployment(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, func(m map[string]any) { workload(m)["name"] = "gone" }))
	c.reconcile(context.Background())
	assert.Contains(t, c.done, "req-1")
	_, real := patches(client)
	assert.Zero(t, real)
}

func TestControllerRejectsFailedDryRun(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	client.PrependReactor("patch", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "web", nil)
	})
	c.reconcile(context.Background())
	assert.Contains(t, c.done, "req-1")
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerRetriesTransientDryRunError(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	first := true
	client.PrependReactor("patch", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		if first {
			first = false
			return true, nil, apierrors.NewInternalError(errors.New("boom"))
		}
		return false, nil, nil
	})

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.NotContains(t, c.done, "req-1", "a transient error is not a rejection")

	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerPatchCarriesResourceVersion(t *testing.T) {
	d := testDeployment(nil)
	d.ResourceVersion = "42"
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	rv := getDeployment(t, client).ResourceVersion // in case the fake tracker overwrote it

	c.reconcile(context.Background())

	var found bool
	for _, a := range client.Actions() {
		p, ok := a.(k8stesting.PatchActionImpl)
		if !ok || len(p.PatchOptions.DryRun) > 0 {
			continue
		}
		found = true
		assert.Contains(t, string(p.Patch), fmt.Sprintf(`"resourceVersion":"%s"`, rv))
	}
	assert.True(t, found, "expected a real (non-dry-run) patch")
}

func TestControllerWaitsForRollout(t *testing.T) {
	d := testDeployment(nil)
	d.Status.UpdatedReplicas = 1
	c, client, _ := newTestController(t, d, rawRequest(t, nil))

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.NotContains(t, c.done, "req-1", "a rollout in progress is not a rejection")

	d = getDeployment(t, client)
	d.Status.UpdatedReplicas = 2
	_, err := client.AppsV1().Deployments("shop").UpdateStatus(context.Background(), d, metav1.UpdateOptions{})
	require.NoError(t, err)
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerRejectsUnsafeRolloutStrategy(t *testing.T) {
	tests := map[string]func(d *appsv1.Deployment){
		"recreate strategy": func(d *appsv1.Deployment) {
			d.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
		},
		"maxUnavailable covers every replica": func(d *appsv1.Deployment) {
			mu := intstr.FromString("100%")
			d.Spec.Strategy = appsv1.DeploymentStrategy{
				Type:          appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{MaxUnavailable: &mu},
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			d := testDeployment(nil)
			mutate(d)
			c, client, _ := newTestController(t, d, rawRequest(t, nil))
			c.reconcile(context.Background())
			assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
			assert.Contains(t, c.done, "req-1")
		})
	}
}

func TestControllerAppliesWithSafeRollingUpdate(t *testing.T) {
	d := testDeployment(nil)
	mu := intstr.FromInt32(1)
	d.Spec.Strategy = appsv1.DeploymentStrategy{
		Type:          appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{MaxUnavailable: &mu},
	}
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.NotContains(t, c.done, "req-1")
}

func TestControllerWaitsWhilePaused(t *testing.T) {
	d := testDeployment(nil)
	d.Spec.Paused = true
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.NotContains(t, c.done, "req-1", "paused is not a rejection")

	d = getDeployment(t, client)
	d.Spec.Paused = false
	_, err := client.AppsV1().Deployments("shop").Update(context.Background(), d, metav1.UpdateOptions{})
	require.NoError(t, err)
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerRevertAndExpiry(t *testing.T) {
	for name, tt := range map[string]struct {
		mutate func(m map[string]any)
		status string
	}{
		"revert":           {func(m map[string]any) { m["action"] = "revert" }, "reverted"},
		"no longer listed": {func(m map[string]any) { m["clusters"] = []any{} }, "reverted"},
		"listed elsewhere": {func(m map[string]any) { m["clusters"].([]any)[0].(map[string]any)["id"] = "cluster-2" }, "reverted"},
		"expired":          {func(m map[string]any) { m["expires_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339) }, "expired"},
		"expired, unlisted": {func(m map[string]any) {
			m["expires_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339)
			m["clusters"] = []any{}
		}, "reverted"},
	} {
		t.Run(name, func(t *testing.T) {
			c, client, _ := newTestController(t, enforced("req-1"), rawRequest(t, tt.mutate))
			c.reconcile(context.Background())

			got := getDeployment(t, client).Spec.Template
			assert.NotContains(t, got.Annotations, RequestsAnnotation)
			assert.Equal(t, map[string]string{"app": "web"}, got.Labels)
			assert.Equal(t, tt.status, c.status["req-1"])
		})
	}
}

func TestControllerRejectsCapabilityOfSomeRuntimesOnly(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, func(m map[string]any) {
		m["parameters"] = map[string]any{"capabilities_add": []string{"NET_RAW"}}
	}))
	c.reconcile(context.Background())

	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.Contains(t, c.status["req-1"], "rejected", "the nodes' runtimes are not known yet")
}

func TestControllerSupersededBeforeApplying(t *testing.T) {
	d := testDeployment(nil)
	d.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	c.reconcile(context.Background())

	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation], "no rollout")
	assert.Contains(t, c.status["req-1"], "superseded")
	_, real := patches(client)
	assert.Zero(t, real)
}

func TestControllerRetireLeavesSupersededInPlace(t *testing.T) {
	d := enforced("req-1")
	d.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"NET_BIND_SERVICE"}},
	}
	c, client, _ := newTestController(t, d, rawRequest(t, func(m map[string]any) { m["clusters"] = []any{} }))
	c.reconcile(context.Background())

	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation], "no rollout once the PR is deployed")
	assert.Contains(t, c.status["req-1"], "superseded")
	_, real := patches(client)
	assert.Zero(t, real)
}

func TestControllerRevertKeepsOtherIDs(t *testing.T) {
	d := testDeployment(map[string]string{RequestsAnnotation: "old,req-1"})
	d.Spec.Template.Labels = map[string]string{EnabledLabel: "true"}
	c, client, _ := newTestController(t, d, rawRequest(t, func(m map[string]any) { m["action"] = "revert" }))
	c.reconcile(context.Background())

	got := getDeployment(t, client).Spec.Template
	assert.Equal(t, "old", got.Annotations[RequestsAnnotation])
	assert.Equal(t, "true", got.Labels[EnabledLabel])
}

func TestControllerDoesNotReAddRemovedRequest(t *testing.T) {
	// kubectl rollout undo restored a template without the keys; the
	// ReplicaSet the trial rolled out is still in the revision history.
	c, client, store := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	create(t, client, testReplicaSet("web-1", time.Hour, "", "web:1.2"), testReplicaSet("web-2", time.Minute, "req-1", "web:1.2"))

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.Contains(t, c.done, "req-1")

	// Nor does a new leader, or a restarted Cluster Agent.
	restarted := NewController(client, store, func() bool { return true })
	restarted.syncWait = 0
	restarted.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerIgnoresAnotherDeploymentsReplicaSets(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	rs := testReplicaSet("web-old", time.Hour, "req-1", "web:1.2")
	rs.OwnerReferences[0].UID = "uid-of-a-deleted-web"
	create(t, client, rs)

	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerSupersededByTemplate(t *testing.T) {
	d := testDeployment(map[string]string{RequestsAnnotation: "req-1"})
	d.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"NET_BIND_SERVICE"}},
	}
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	c.reconcile(context.Background())

	assert.Contains(t, c.done, "req-1")
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation], "superseded in place, without a restart")
	_, real := patches(client)
	assert.Zero(t, real)
}

// watched returns a controller watching req-1, applied on enforced("req-1")
// by a ReplicaSet created age ago, whose pod reports status.
func watched(t *testing.T, d *appsv1.Deployment, age time.Duration, status corev1.ContainerStatus, reqs ...[]byte) (*Controller, *fake.Clientset) {
	if len(reqs) == 0 {
		reqs = [][]byte{rawRequest(t, nil)}
	}
	c, client, _ := newTestController(t, d, reqs...)
	rs := rollout(d, "web-2", age)
	create(t, client, rs, hardenedPod("web-2-a", rs, status))
	return c, client
}

func TestControllerRollsBackInsideWindow(t *testing.T) {
	for name, tt := range map[string]struct {
		condition appsv1.DeploymentCondition
		status    corev1.ContainerStatus
	}{
		"restart":                  {status: restarted("Error")},
		"liveness kill":            {status: restarted("Completed")},
		"ProgressDeadlineExceeded": {condition: appsv1.DeploymentCondition{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"}},
		"Available=False":          {condition: appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse}},
	} {
		t.Run(name, func(t *testing.T) {
			d := enforced("req-1")
			if tt.condition.Type != "" {
				d.Status.Conditions = []appsv1.DeploymentCondition{tt.condition}
			}
			c, client := watched(t, d, 10*time.Minute, tt.status)
			c.reconcile(context.Background())

			got := getDeployment(t, client).Spec.Template
			assert.NotContains(t, got.Annotations, RequestsAnnotation)
			assert.NotContains(t, got.Labels, EnabledLabel)
			assert.Contains(t, c.status["req-1"], "rolled_back")
			assert.Contains(t, c.done, "req-1")
		})
	}
}

func TestControllerIgnoresStaleConditions(t *testing.T) {
	d := enforced("req-1")
	d.Generation = 2 // the Deployment controller has not observed the template change yet
	d.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse}}
	c, client := watched(t, d, time.Minute, corev1.ContainerStatus{})
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerKeepsRequestThrough(t *testing.T) {
	for name, tt := range map[string]struct {
		age    time.Duration
		status corev1.ContainerStatus
		want   string
	}{
		"an OOM kill":                {age: 10 * time.Minute, status: restarted("OOMKilled"), want: "applied"},
		"a healthy window":           {age: 10 * time.Minute, want: "applied"},
		"the end of a clean window":  {age: 2 * time.Hour, want: "confirmed"},
		"a restart after the window": {age: 2 * time.Hour, status: restarted("Error"), want: "degraded: container app of pod web-2-a restarted (Error, exit code 1)"},
	} {
		t.Run(name, func(t *testing.T) {
			c, client := watched(t, enforced("req-1"), tt.age, tt.status)
			c.status["req-1"] = "applied"
			c.reconcile(context.Background())

			assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
			assert.Equal(t, tt.want, c.status["req-1"])
			assert.NotContains(t, c.done, "req-1", "an enforced request stays watched")
		})
	}
}

func TestControllerNeverRevertsDuringCustomerRollout(t *testing.T) {
	d := enforced("req-1")
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	ours := rollout(d, "web-2", 10*time.Minute)
	d.Spec.Template.Spec.Containers[0].Image = "web:1.3" // a new release keeps the keys
	theirs := rollout(d, "web-3", time.Minute)
	create(t, client, ours, theirs, hardenedPod("web-3-a", theirs, restarted("Error")))
	_, err := client.AppsV1().Deployments("shop").Update(context.Background(), d, metav1.UpdateOptions{})
	require.NoError(t, err)

	c.reconcile(context.Background())

	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation], "a new release stays hardened")
	assert.Contains(t, c.status["req-1"], "degraded")
}

func TestControllerReportsUnsafeCustomerChange(t *testing.T) {
	d := enforced("req-1")
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	ours := rollout(d, "web-2", 10*time.Minute)
	d.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
	create(t, client, ours, rollout(d, "web-3", time.Minute))
	_, err := client.AppsV1().Deployments("shop").Update(context.Background(), d, metav1.UpdateOptions{})
	require.NoError(t, err)

	c.reconcile(context.Background())
	assert.Contains(t, c.status["req-1"], "degraded: the customer's template change makes the rollout unsafe to watch")
}

func TestControllerReportsPartial(t *testing.T) {
	d := enforced("req-1")
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	rs := rollout(d, "web-2", 10*time.Minute)
	unhardened := hardenedPod("web-2-b", rs, corev1.ContainerStatus{})
	unhardened.Annotations[AppliedAnnotation] = `{"req-1":"skipped: unknown request"}`
	create(t, client, rs, unhardened)

	c.reconcile(context.Background())
	assert.Equal(t, `partial: pod web-2-b started without the settings ("skipped: unknown request")`, c.status["req-1"])
}

// TestControllerPinsFailures covers two requests on one container, both in
// their confirmation window.
func TestControllerPinsFailures(t *testing.T) {
	ro := rawRequest(t, func(m map[string]any) {
		m["id"], m["control"], m["parameters"] = "req-2", "read_only_root_fs", map[string]any{}
	})
	for name, tt := range map[string]struct {
		rollingOut bool
		want       string
	}{
		"during req-2's rollout": {rollingOut: true, want: "req-1"},
		"after the rollouts":     {want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			d := enforced("req-1")
			first := rollout(d, "web-2", 30*time.Minute)
			d.Spec.Template.Annotations[RequestsAnnotation] = "req-1,req-2"
			second := rollout(d, "web-3", 5*time.Minute)
			if tt.rollingOut {
				d.Status.UpdatedReplicas = 1
			}
			c, client, _ := newTestController(t, d, rawRequest(t, nil), ro)
			create(t, client, first, second, hardenedPod("web-3-a", second, restarted("Error")))

			c.reconcile(context.Background())

			assert.Equal(t, tt.want, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
			assert.Contains(t, c.status["req-2"], "rolled_back")
			assert.Equal(t, tt.want == "", strings.Contains(c.status["req-1"], "rolled_back"))
		})
	}
}

func TestControllerCapsRolloutsInProgress(t *testing.T) {
	others := [][]byte{rawRequest(t, nil)}
	for _, id := range []string{"a", "b", "c"} {
		others = append(others, rawRequest(t, func(m map[string]any) { m["id"] = id; workload(m)["name"] = id }))
	}
	c, client, _ := newTestController(t, testDeployment(nil), others...)
	c.rollingOut = map[string]bool{"a": true, "b": true, "c": true}

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.NotContains(t, c.done, "req-1", "a cap is a wait, not a rejection")

	delete(c.rollingOut, "a")
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerCapsAppliesPerHour(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	now := time.Now()
	c.now = func() time.Time { return now }
	for range maxAppliesPerHour {
		c.applied = append(c.applied, now.Add(-59*time.Minute))
	}

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])

	now = now.Add(time.Minute)
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerHeartbeat(t *testing.T) {
	c, _, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	now := time.Now()
	c.now = func() time.Time { return now }
	c.reconcile(context.Background())
	first := c.lastHeartbeat

	now = now.Add(30 * time.Minute)
	c.reconcile(context.Background())
	assert.Equal(t, first, c.lastHeartbeat)

	now = now.Add(31 * time.Minute)
	c.reconcile(context.Background())
	assert.Equal(t, now, c.lastHeartbeat)
}

func TestControllerDeletedConfigIsInert(t *testing.T) {
	c, client, store := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	c.reconcile(context.Background())

	setRC(t, store) // every cluster is superseded: cws-api deletes the config
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerNotLeader(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	c.isLeader = func() bool { return false }
	c.reconcile(context.Background())
	assert.Empty(t, client.Actions())
}

func TestControllerRunReconcilesOnceThenStopsOnAlreadyCancelledContext(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c.Run(ctx)

	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation],
		"the in-flight reconcile pass completes even though ctx is already done")
}

func TestStartRequiresRCOrRequestsFile(t *testing.T) {
	_, err := Start(context.Background(), "", testCluster, fake.NewClientset(), func() bool { return true }, nil)
	assert.Error(t, err)
}

func TestStartWithRequestsFileReturnsStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // so the WatchFile and Run goroutines exit right away
	path := filepath.Join(t.TempDir(), "requests.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0o600))

	store, err := Start(ctx, path, testCluster, fake.NewClientset(), func() bool { return true }, nil)
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestRefreshDefaults(t *testing.T) {
	node := func(name, runtime string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{ContainerRuntimeVersion: runtime}},
		}
	}
	docker := Defaults{Capabilities: dockerCapabilities, Exact: true}
	ctx := context.Background()
	store := NewStore(testCluster)
	assert.Equal(t, fallbackDefaults, store.Defaults(), "the intersection until the nodes are listed")

	client := fake.NewClientset(node("a", "containerd://1.7.2"), node("b", "containerd://2.0.0"))
	refreshDefaults(ctx, client, store)
	assert.Equal(t, docker, store.Defaults())

	client.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})
	refreshDefaults(ctx, client, store)
	assert.Equal(t, docker, store.Defaults(), "a failed listing keeps the previous defaults")

	refreshDefaults(ctx, fake.NewClientset(node("a", "containerd://1.7.2"), node("c", "cri-o://1.29.1")), store)
	assert.Equal(t, fallbackDefaults, store.Defaults())
}
