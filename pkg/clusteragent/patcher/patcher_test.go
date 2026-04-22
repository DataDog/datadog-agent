// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patcher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newFakeClient creates a fake dynamic client with the apps/v1 and core/v1
// resource schemes registered.
func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			deploymentGVR:  "DeploymentList",
			statefulSetGVR: "StatefulSetList",
			podGVR:         "PodList",
		},
		objects...,
	)
}

// newUnstructuredDeployment creates an unstructured Deployment for testing.
func newUnstructuredDeployment(namespace, name string, metadataAnnotations, templateAnnotations map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{},
					"spec":     map[string]interface{}{},
				},
			},
		},
	}
	if metadataAnnotations != nil {
		obj.Object["metadata"].(map[string]interface{})["annotations"] = metadataAnnotations
	}
	if templateAnnotations != nil {
		obj.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["metadata"].(map[string]interface{})["annotations"] = templateAnnotations
	}
	return obj
}

// newUnstructuredPod creates an unstructured Pod for testing.
func newUnstructuredPod(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{},
		},
	}
}

func TestPatcherApplyMetadataAnnotations(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"test-key": "test-value",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)

	// Verify the patch was applied by reading the resource back
	result, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)

	annotations := result.GetAnnotations()
	assert.Equal(t, "test-value", annotations["test-key"])
}

func TestPatcherApplyPodTemplateAnnotations(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetPodTemplateAnnotations(map[string]interface{}{
			"template-key": "template-value",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)

	result, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)

	// Navigate to spec.template.metadata.annotations
	spec := result.Object["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	metadata := template["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "template-value", annotations["template-key"])
}

func TestPatcherApplyPodAnnotations(t *testing.T) {
	pod := newUnstructuredPod("default", "my-pod")
	client := newFakeClient(pod)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(PodTarget("default", "my-pod")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"pod-key": "pod-value",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)

	result, err := client.Resource(podGVR).Namespace("default").Get(context.Background(), "my-pod", metav1.GetOptions{})
	require.NoError(t, err)

	annotations := result.GetAnnotations()
	assert.Equal(t, "pod-value", annotations["pod-key"])
}

func TestPatcherApplyEmptyIntent(t *testing.T) {
	client := newFakeClient()
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy"))
	// No operations added

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.False(t, applied, "empty intent should be a no-op")
}

func TestPatcherApplyNonExistentResource(t *testing.T) {
	client := newFakeClient() // No objects
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "does-not-exist")).
		With(SetMetadataAnnotations(map[string]interface{}{"k": "v"}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	assert.False(t, applied)
	assert.Error(t, err)
}

func TestPatcherApplyMultipleOperations(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"rc.id":  "config-123",
			"rc.rev": "42",
		})).
		With(SetPodTemplateLabels(map[string]interface{}{
			"admission.datadoghq.com/enabled": "true",
		})).
		With(SetPodTemplateAnnotations(map[string]interface{}{
			"java-lib.version": "v1.4.0",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)

	result, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)

	// Verify metadata annotations
	annotations := result.GetAnnotations()
	assert.Equal(t, "config-123", annotations["rc.id"])
	assert.Equal(t, "42", annotations["rc.rev"])

	// Verify template labels and annotations
	spec := result.Object["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	metadata := template["metadata"].(map[string]interface{})

	templateLabels := metadata["labels"].(map[string]interface{})
	assert.Equal(t, "true", templateLabels["admission.datadoghq.com/enabled"])

	templateAnnotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "v1.4.0", templateAnnotations["java-lib.version"])
}

func TestPatcherApplyDeleteAnnotations(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, map[string]interface{}{
		"keep-me":   "value",
		"remove-me": "old-value",
	})
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(DeletePodTemplateAnnotations([]string{"remove-me", "other-remove-nonexistent"}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)

	result, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)

	rspec := result.Object["spec"].(map[string]interface{})
	rtemplate := rspec["template"].(map[string]interface{})
	rmetadata := rtemplate["metadata"].(map[string]interface{})
	rAnnotations := rmetadata["annotations"].(map[string]interface{})

	assert.Equal(t, "value", rAnnotations["keep-me"])
	// With merge patch, nil should remove the key
	_, exists := rAnnotations["remove-me"]
	assert.False(t, exists, "deleted annotation should be removed")
}

func TestPatcherApplyPreservesExistingAnnotations(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", map[string]interface{}{
		"existing-key": "existing-value",
	}, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"new-key": "new-value",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)

	result, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)

	annotations := result.GetAnnotations()
	assert.Equal(t, "existing-value", annotations["existing-key"], "existing annotation should be preserved")
	assert.Equal(t, "new-value", annotations["new-key"], "new annotation should be added")
}

func TestPatcherApplyDryRun(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"dry-run-key": "dry-run-value",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test", DryRun: true, RetryOnConflict: true})
	require.NoError(t, err)
	assert.True(t, applied)

	// Verify a patch action was issued (the fake client doesn't enforce
	// DryRun semantics, but we verify the patch was constructed)
	actions := client.Actions()
	require.NotEmpty(t, actions)
	lastAction := actions[len(actions)-1]
	patchAction, ok := lastAction.(k8stesting.PatchAction)
	require.True(t, ok)
	assert.NotEmpty(t, patchAction.GetPatch())
}

func TestPatcherApplyUsesCorrectPatchType(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{"k": "v"}))

	_, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)

	// Verify MergePatchType was used (default patch type)
	actions := client.Actions()
	require.NotEmpty(t, actions)
	lastAction := actions[len(actions)-1]
	patchAction, ok := lastAction.(k8stesting.PatchAction)
	require.True(t, ok, "last action should be a patch")
	assert.Equal(t, string(types.MergePatchType), string(patchAction.GetPatchType()))
}

func TestPatcherApplyBuildsCorrectJSON(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"rc.id": "cfg-1",
		})).
		With(SetPodTemplateAnnotations(map[string]interface{}{
			"version": "v1",
		}))

	_, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)

	actions := client.Actions()
	patchAction := actions[len(actions)-1].(k8stesting.PatchAction)
	patchBytes := patchAction.GetPatch()

	var patchBody map[string]interface{}
	err = json.Unmarshal(patchBytes, &patchBody)
	require.NoError(t, err)

	// Verify top-level metadata
	metadata := patchBody["metadata"].(map[string]interface{})
	metaAnnotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "cfg-1", metaAnnotations["rc.id"])

	// Verify template annotations
	spec := patchBody["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	templateMeta := template["metadata"].(map[string]interface{})
	templateAnnotations := templateMeta["annotations"].(map[string]interface{})
	assert.Equal(t, "v1", templateAnnotations["version"])
}

func TestPatcherRespondsToLeadershipChange(t *testing.T) {
	deploy := newUnstructuredDeployment("default", "my-deploy", nil, nil)
	client := newFakeClient(deploy)

	isLeader := false
	p := NewPatcher(client, func() bool { return isLeader })

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{"k": "v"}))

	// Not leader — should skip
	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.False(t, applied)

	// Become leader — should apply
	isLeader = true
	applied, err = p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)
	assert.True(t, applied)
}

// --- Scenario tests matching real use cases ---

func TestScenarioRCPatcherEnable(t *testing.T) {
	// Simulates the RC patcher enable flow
	deploy := newUnstructuredDeployment("default", "my-java-service", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-java-service")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"admission.datadoghq.com/rc.id":  "config-abc",
			"admission.datadoghq.com/rc.rev": "1674236639474287600",
		})).
		With(SetPodTemplateLabels(map[string]interface{}{
			"admission.datadoghq.com/enabled": "true",
		})).
		With(SetPodTemplateAnnotations(map[string]interface{}{
			"admission.datadoghq.com/java-lib.version":   "v1.4.0",
			"admission.datadoghq.com/java-lib.config.v1": `{"service_name":"my-app"}`,
			"admission.datadoghq.com/rc.id":              "config-abc",
			"admission.datadoghq.com/rc.rev":             "1674236639474287600",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "rc_patcher"})
	require.NoError(t, err)
	assert.True(t, applied)
}

func TestScenarioLanguageDetection(t *testing.T) {
	// Simulates the language detection patcher flow
	deploy := newUnstructuredDeployment("default", "my-deploy", map[string]interface{}{
		"internal.dd.datadoghq.com/app.detected_langs":   "java",
		"internal.dd.datadoghq.com/stale.detected_langs": "python",
	}, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("default", "my-deploy")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"internal.dd.datadoghq.com/app.detected_langs":   "java,python",
			"internal.dd.datadoghq.com/stale.detected_langs": nil, // remove stale
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{
		Caller:          "language_detection",
		RetryOnConflict: true,
	})
	require.NoError(t, err)
	assert.True(t, applied)

	result, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)

	annotations := result.GetAnnotations()
	assert.Equal(t, "java,python", annotations["internal.dd.datadoghq.com/app.detected_langs"])
	_, exists := annotations["internal.dd.datadoghq.com/stale.detected_langs"]
	assert.False(t, exists, "stale annotation should be removed")
}

func TestScenarioVPARollout(t *testing.T) {
	// Simulates the VPA vertical controller rollout trigger
	deploy := newUnstructuredDeployment("ns1", "web-app", nil, nil)
	client := newFakeClient(deploy)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(DeploymentTarget("ns1", "web-app")).
		With(SetPodTemplateAnnotations(map[string]interface{}{
			"autoscaling.datadoghq.com/rollout-timestamp": "2024-01-15T10:30:00Z",
			"autoscaling.datadoghq.com/scaling-hash":      "abc123def",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "vpa"})
	require.NoError(t, err)
	assert.True(t, applied)
}

func TestPatcherApplySubresource(t *testing.T) {
	pod := newUnstructuredPod("default", "my-pod")
	client := newFakeClient(pod)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(PodTarget("default", "my-pod")).
		With(SetMetadataAnnotations(map[string]interface{}{"k": "v"}))

	_, err := p.Apply(context.Background(), intent, PatchOptions{
		Caller:      "test",
		Subresource: "resize",
	})
	require.NoError(t, err)

	actions := client.Actions()
	require.NotEmpty(t, actions)
	lastAction := actions[len(actions)-1]
	patchAction, ok := lastAction.(k8stesting.PatchAction)
	require.True(t, ok)
	assert.Equal(t, "resize", patchAction.GetSubresource())
}

func TestPatcherApplyNoSubresource(t *testing.T) {
	pod := newUnstructuredPod("default", "my-pod")
	client := newFakeClient(pod)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(PodTarget("default", "my-pod")).
		With(SetMetadataAnnotations(map[string]interface{}{"k": "v"}))

	_, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "test"})
	require.NoError(t, err)

	actions := client.Actions()
	require.NotEmpty(t, actions)
	lastAction := actions[len(actions)-1]
	patchAction, ok := lastAction.(k8stesting.PatchAction)
	require.True(t, ok)
	assert.Equal(t, "", patchAction.GetSubresource(), "no subresource should be set when Subresource is empty")
}

func TestScenarioVPAPodPatcher(t *testing.T) {
	// Simulates the VPA pod patcher annotation update
	pod := newUnstructuredPod("ns1", "web-app-abc123")
	client := newFakeClient(pod)
	p := NewPatcher(client, nil)

	intent := NewPatchIntent(PodTarget("ns1", "web-app-abc123")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"autoscaling.datadoghq.com/recommendation-applied-event-generated": "true",
		}))

	applied, err := p.Apply(context.Background(), intent, PatchOptions{Caller: "pod_patcher"})
	require.NoError(t, err)
	assert.True(t, applied)
}
