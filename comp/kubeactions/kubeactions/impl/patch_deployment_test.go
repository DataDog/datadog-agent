// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// mustJSON marshals v to a json.RawMessage, panicking on error.
func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func patchInputs(namespace, name, resourceID string, patch json.RawMessage, strategy string) kubeactions.PatchDeploymentInputs {
	return kubeactions.PatchDeploymentInputs{
		ResourceRef: kubeactions.ResourceRef{
			Kind:       "Deployment",
			Namespace:  namespace,
			Name:       name,
			ResourceID: resourceID,
		},
		Patch:         patch,
		PatchStrategy: strategy,
	}
}

func TestPatchDeploymentExecutor_ScaleReplicas(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("test-uid-123"),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "test-uid-123",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)
	assert.Contains(t, result.Message, "patched")
}

func TestPatchDeploymentExecutor_UIDMismatch(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("real-uid"),
		},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "wrong-uid",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "UID mismatch")
}

func TestPatchDeploymentExecutor_MissingNamespace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("", "my-deployment", "test-uid",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	// Namespace validation happens in the validator layer before the executor is called.
	// With an empty namespace, the k8s API returns a not-found error.
	assert.Contains(t, result.Message, "not found")
}

func TestPatchDeploymentExecutor_MissingPatch(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "test-uid", nil, "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "patch is required")
}

func TestPatchDeploymentExecutor_WithMergeStrategy(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("test-uid-merge"),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "test-uid-merge",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 10}}), "merge")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)
	assert.Contains(t, result.Message, "patched")
}

func TestResolvePatchType(t *testing.T) {
	assert.Equal(t, k8stypes.StrategicMergePatchType, resolvePatchType(""))
	assert.Equal(t, k8stypes.StrategicMergePatchType, resolvePatchType("strategic-merge"))
	assert.Equal(t, k8stypes.MergePatchType, resolvePatchType("merge"))
	assert.Equal(t, k8stypes.JSONPatchType, resolvePatchType("json"))
	assert.Equal(t, k8stypes.StrategicMergePatchType, resolvePatchType("unknown"))
}

func TestPatchDeploymentExecutor_DeploymentNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset() // empty — no deployments
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "nonexistent", "test-uid",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "failed to get deployment")
}

func TestApplyUIDGuard(t *testing.T) {
	t.Run("empty uid leaves the patch unchanged", func(t *testing.T) {
		in := mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}})
		out, err := applyUIDGuard(k8stypes.StrategicMergePatchType, in, "")
		require.NoError(t, err)
		assert.JSONEq(t, string(in), string(out))
	})

	t.Run("strategic/merge patch gets metadata.uid injected", func(t *testing.T) {
		for _, pt := range []k8stypes.PatchType{k8stypes.StrategicMergePatchType, k8stypes.MergePatchType} {
			out, err := applyUIDGuard(pt, mustJSON(map[string]interface{}{
				"metadata": map[string]interface{}{"labels": map[string]string{"k": "v"}},
				"spec":     map[string]interface{}{"replicas": 5},
			}), "uid-abc")
			require.NoError(t, err)

			var obj map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out, &obj))
			var meta map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(obj["metadata"], &meta))
			// uid guard added without dropping the caller's existing metadata.
			assert.JSONEq(t, `"uid-abc"`, string(meta["uid"]))
			assert.Contains(t, string(meta["labels"]), "\"k\":\"v\"")
			assert.Contains(t, string(obj["spec"]), "\"replicas\":5")
		}
	})

	t.Run("json patch gets a test op on /metadata/uid prepended", func(t *testing.T) {
		out, err := applyUIDGuard(k8stypes.JSONPatchType, mustJSON([]map[string]interface{}{
			{"op": "replace", "path": "/spec/replicas", "value": 5},
		}), "uid-abc")
		require.NoError(t, err)

		var ops []map[string]interface{}
		require.NoError(t, json.Unmarshal(out, &ops))
		require.Len(t, ops, 2)
		assert.Equal(t, "test", ops[0]["op"])
		assert.Equal(t, "/metadata/uid", ops[0]["path"])
		assert.Equal(t, "uid-abc", ops[0]["value"])
		// the caller's op is preserved after the guard.
		assert.Equal(t, "replace", ops[1]["op"])
	})

	t.Run("malformed patch is rejected", func(t *testing.T) {
		_, err := applyUIDGuard(k8stypes.MergePatchType, json.RawMessage("not json"), "uid-abc")
		assert.Error(t, err)
		_, err = applyUIDGuard(k8stypes.JSONPatchType, json.RawMessage("{}"), "uid-abc")
		assert.Error(t, err)
	})
}
