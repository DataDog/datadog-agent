// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

var deploymentGVK = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

func newScalerForTest(t *testing.T, objs ...runtime.Object) (*scalerImpl, *dynamicfake.FakeDynamicClient) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, objs...)

	mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "apps", Version: "v1"}})
	mapper.Add(deploymentGVK, apimeta.RESTScopeNamespace)

	return &scalerImpl{
		restMapper:    mapper,
		dynamicClient: dynClient,
	}, dynClient
}

func newDeploymentWithManagedFields(namespace, name string, managedFields []metav1.ManagedFieldsEntry) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:     namespace,
			Name:          name,
			ManagedFields: managedFields,
		},
	}
}

func TestScalerReleaseReplicasOwnership_RemovesClusterAgentEntry(t *testing.T) {
	otherEntry := metav1.ManagedFieldsEntry{
		Manager:   "kubectl-edit",
		Operation: metav1.ManagedFieldsOperationApply,
	}
	helmEntry := metav1.ManagedFieldsEntry{
		Manager:   "helm",
		Operation: metav1.ManagedFieldsOperationApply,
	}
	dcaEntry := metav1.ManagedFieldsEntry{
		Manager:     datadogClusterAgentFieldManager,
		Operation:   metav1.ManagedFieldsOperationUpdate,
		Subresource: "scale",
	}

	deployment := newDeploymentWithManagedFields("default", "app",
		[]metav1.ManagedFieldsEntry{otherEntry, dcaEntry, helmEntry})

	sg, dynClient := newScalerForTest(t, deployment)

	require.NoError(t, sg.releaseReplicasOwnership(context.Background(), "default", "app", deploymentGVK))

	got, err := dynClient.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).
		Namespace("default").Get(context.Background(), "app", metav1.GetOptions{})
	require.NoError(t, err)

	managers := []string{}
	for _, mf := range got.GetManagedFields() {
		managers = append(managers, mf.Manager)
	}
	assert.ElementsMatch(t, []string{"kubectl-edit", "helm"}, managers, "DCA scale entry should be removed; other entries preserved")
}

func TestScalerReleaseReplicasOwnership_NoEntryIsNoOp(t *testing.T) {
	// No managedFields entry from the cluster agent — release should be a no-op.
	deployment := newDeploymentWithManagedFields("default", "app",
		[]metav1.ManagedFieldsEntry{{Manager: "helm", Operation: metav1.ManagedFieldsOperationApply}})

	sg, dynClient := newScalerForTest(t, deployment)

	// Reset action tracking to ensure we observe only what releaseReplicasOwnership triggers.
	dynClient.ClearActions()
	require.NoError(t, sg.releaseReplicasOwnership(context.Background(), "default", "app", deploymentGVK))

	for _, action := range dynClient.Actions() {
		assert.NotEqual(t, "patch", action.GetVerb(), "no patch should be issued when no DCA entry exists")
	}
}

func TestScalerReleaseReplicasOwnership_TargetNotFoundIsNoError(t *testing.T) {
	// No deployment created; the dynamic client returns NotFound on Get.
	sg, _ := newScalerForTest(t)

	err := sg.releaseReplicasOwnership(context.Background(), "default", "missing", deploymentGVK)
	assert.NoError(t, err, "missing target should be treated as already-released")
}

func TestScalerReleaseReplicasOwnership_ForbiddenSurfacesEvenWithMultipleMappings(t *testing.T) {
	// RESTMapper iteration was historically prone to silently returning nil
	// when a later mapping returned NotFound after an earlier mapping had
	// returned a real error (Forbidden). The fix collapses to a single
	// (preferred) mapping. Assert that a Forbidden on the first mapping
	// propagates rather than being swallowed.
	sg, dynClient := newScalerForTest(t)
	dynClient.PrependReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(
			schema.GroupResource{Group: "apps", Resource: "deployments"},
			"app", errors.New("user cannot patch deployments.apps"))
	})

	err := sg.releaseReplicasOwnership(context.Background(), "default", "app", deploymentGVK)
	assert.Error(t, err, "Forbidden must propagate so the caller can requeue / surface it")
	assert.True(t, k8serrors.IsForbidden(err) || strings.Contains(err.Error(), "forbidden"),
		"underlying Forbidden must be preserved (errors.Is or string contains)")
}

func TestScalerReleaseReplicasOwnership_PatchIsTestGuarded(t *testing.T) {
	// Each remove op must be preceded by `test` ops asserting the entry at
	// that index still has manager=datadog-cluster-agent and
	// subresource=scale. This is the only thing that keeps a concurrent
	// writer (Helm SSA, kubectl, another controller) shifting managedFields
	// between our GET and PATCH from causing us to drop a foreign entry by
	// stale index. Verify the emitted patch body has the expected shape.
	dcaEntry := metav1.ManagedFieldsEntry{
		Manager:     datadogClusterAgentFieldManager,
		Operation:   metav1.ManagedFieldsOperationUpdate,
		Subresource: "scale",
	}
	helmEntry := metav1.ManagedFieldsEntry{
		Manager:   "helm",
		Operation: metav1.ManagedFieldsOperationApply,
	}
	deployment := newDeploymentWithManagedFields("default", "app",
		[]metav1.ManagedFieldsEntry{helmEntry, dcaEntry})

	sg, dynClient := newScalerForTest(t, deployment)
	dynClient.ClearActions()
	require.NoError(t, sg.releaseReplicasOwnership(context.Background(), "default", "app", deploymentGVK))

	var patchBody []byte
	for _, action := range dynClient.Actions() {
		if action.GetVerb() != "patch" {
			continue
		}
		patchAction, ok := action.(k8stesting.PatchAction)
		require.True(t, ok, "patch action should implement PatchAction")
		patchBody = patchAction.GetPatch()
	}
	require.NotNil(t, patchBody, "expected one patch action")

	var ops []map[string]any
	require.NoError(t, json.Unmarshal(patchBody, &ops))
	require.Len(t, ops, 3, "expected 3 ops (test manager, test subresource, remove) for the single DCA entry")
	assert.Equal(t, "test", ops[0]["op"])
	assert.Equal(t, "/metadata/managedFields/1/manager", ops[0]["path"])
	assert.Equal(t, datadogClusterAgentFieldManager, ops[0]["value"])
	assert.Equal(t, "test", ops[1]["op"])
	assert.Equal(t, "/metadata/managedFields/1/subresource", ops[1]["path"])
	assert.Equal(t, "scale", ops[1]["value"])
	assert.Equal(t, "remove", ops[2]["op"])
	assert.Equal(t, "/metadata/managedFields/1", ops[2]["path"])
}

func TestScalerReleaseReplicasOwnership_TestOpFailureRejectsPatch(t *testing.T) {
	// Simulate the race the test ops guard against: our GET sees the DCA
	// entry at index 1, but the underlying store has been mutated so index
	// 1 now points at a different manager. The `test` op against that
	// index must fail and the whole patch must be rejected atomically —
	// returning an error rather than silently dropping a foreign entry by
	// stale index. The next reconcile retries with a fresh snapshot.
	dcaEntry := metav1.ManagedFieldsEntry{
		Manager:     datadogClusterAgentFieldManager,
		Operation:   metav1.ManagedFieldsOperationUpdate,
		Subresource: "scale",
	}
	helmEntry := metav1.ManagedFieldsEntry{
		Manager:   "helm",
		Operation: metav1.ManagedFieldsOperationApply,
	}
	other := metav1.ManagedFieldsEntry{
		Manager:   "kubectl-edit",
		Operation: metav1.ManagedFieldsOperationUpdate,
	}

	// What our GET will return: [helm, dca-scale] (DCA at index 1).
	getView := newDeploymentWithManagedFields("default", "app",
		[]metav1.ManagedFieldsEntry{helmEntry, dcaEntry})
	// What the underlying store actually holds: [kubectl-edit, helm, dca-scale]
	// (a concurrent writer inserted at the front; DCA is now at index 2,
	// helm is at index 1).
	storedView := newDeploymentWithManagedFields("default", "app",
		[]metav1.ManagedFieldsEntry{other, helmEntry, dcaEntry})

	sg, dynClient := newScalerForTest(t, storedView)
	// Override Get to return the stale snapshot. PATCH still runs against
	// the underlying store, so the test op asserting manager at index 1
	// equals "datadog-cluster-agent" will fail (it's now "helm").
	dynClient.PrependReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(getView)
		if err != nil {
			return true, nil, err
		}
		return true, &unstructured.Unstructured{Object: u}, nil
	})

	err := sg.releaseReplicasOwnership(context.Background(), "default", "app", deploymentGVK)
	assert.Error(t, err, "patch must be rejected when managedFields shifted between GET and PATCH")
}

func TestScalerReleaseReplicasOwnership_IgnoresNonScaleEntries(t *testing.T) {
	// An entry owned by the cluster agent but NOT on the scale subresource
	// (e.g. a parent-resource update) must be left untouched. Only the
	// scale-subresource entry is the one that conflicts with Helm SSA on
	// `.spec.replicas`.
	dcaParent := metav1.ManagedFieldsEntry{
		Manager:   datadogClusterAgentFieldManager,
		Operation: metav1.ManagedFieldsOperationUpdate,
	}
	deployment := newDeploymentWithManagedFields("default", "app",
		[]metav1.ManagedFieldsEntry{dcaParent})

	sg, dynClient := newScalerForTest(t, deployment)

	require.NoError(t, sg.releaseReplicasOwnership(context.Background(), "default", "app", deploymentGVK))

	got, err := dynClient.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).
		Namespace("default").Get(context.Background(), "app", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, got.GetManagedFields(), 1, "non-scale DCA entry must be preserved")
	assert.Equal(t, datadogClusterAgentFieldManager, got.GetManagedFields()[0].Manager)
	assert.Empty(t, got.GetManagedFields()[0].Subresource)
}
