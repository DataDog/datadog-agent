// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
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
	return NewController(client, store, func() bool { return true }), client, store
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

func TestControllerAppliesTrial(t *testing.T) {
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
		"uid mismatch": func(d *appsv1.Deployment) { d.UID = "uid-2" },
		"privileged": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptr.To(true)}
		},
		"missing container": func(d *appsv1.Deployment) { d.Spec.Template.Spec.Containers[0].Name = "other" },
		"not narrowing": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			d := testDeployment(nil)
			mutate(d)
			c, client, _ := newTestController(t, d, rawRequest(t, nil))
			c.reconcile(context.Background())
			assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
			assert.Contains(t, c.rejected, "req-1")
		})
	}
}

func TestControllerRejectsMissingDeployment(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, func(m map[string]any) { target(m)["name"] = "gone" }))
	c.reconcile(context.Background())
	assert.Contains(t, c.rejected, "req-1")
	_, real := patches(client)
	assert.Zero(t, real)
}

func TestControllerRejectsFailedDryRun(t *testing.T) {
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	client.PrependReactor("patch", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "web", nil)
	})
	c.reconcile(context.Background())
	assert.Contains(t, c.rejected, "req-1")
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
	assert.NotContains(t, c.rejected, "req-1", "a transient error is not a rejection")

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
	assert.NotContains(t, c.rejected, "req-1", "a rollout in progress is not a rejection")

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
			assert.Contains(t, c.rejected, "req-1")
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
	assert.NotContains(t, c.rejected, "req-1")
}

func TestControllerWaitsWhilePaused(t *testing.T) {
	d := testDeployment(nil)
	d.Spec.Paused = true
	c, client, _ := newTestController(t, d, rawRequest(t, nil))
	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
	assert.NotContains(t, c.rejected, "req-1", "paused is not a rejection")

	d = getDeployment(t, client)
	d.Spec.Paused = false
	_, err := client.AppsV1().Deployments("shop").Update(context.Background(), d, metav1.UpdateOptions{})
	require.NoError(t, err)
	c.reconcile(context.Background())
	assert.Equal(t, "req-1", getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerRevertAndExpiry(t *testing.T) {
	for name, mutate := range map[string]func(m map[string]any){
		"revert":  func(m map[string]any) { m["action"] = "revert" },
		"expired": func(m map[string]any) { m["expires_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339) },
	} {
		t.Run(name, func(t *testing.T) {
			d := testDeployment(map[string]string{RequestsAnnotation: "req-1"})
			d.Spec.Template.Labels = map[string]string{EnabledLabel: "true", "app": "web"}
			c, client, _ := newTestController(t, d, rawRequest(t, mutate))
			c.reconcile(context.Background())

			got := getDeployment(t, client).Spec.Template
			assert.NotContains(t, got.Annotations, RequestsAnnotation)
			assert.Equal(t, map[string]string{"app": "web"}, got.Labels)
		})
	}
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
	c, client, _ := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	c.reconcile(context.Background())

	// kubectl rollout undo restores a template without the keys.
	d := getDeployment(t, client)
	d.Spec.Template.Annotations = nil
	d.Spec.Template.Labels = nil
	_, err := client.AppsV1().Deployments("shop").Update(context.Background(), d, metav1.UpdateOptions{})
	require.NoError(t, err)

	c.reconcile(context.Background())
	assert.Empty(t, getDeployment(t, client).Spec.Template.Annotations[RequestsAnnotation])
}

func TestControllerDeletedConfigIsInert(t *testing.T) {
	c, client, store := newTestController(t, testDeployment(nil), rawRequest(t, nil))
	c.reconcile(context.Background())

	setRC(t, store) // PR merged: cws-api deletes the config
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
