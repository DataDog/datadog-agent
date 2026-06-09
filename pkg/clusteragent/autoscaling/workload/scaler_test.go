// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
