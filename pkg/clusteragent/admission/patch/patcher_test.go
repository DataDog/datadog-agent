// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPatchDeployment(t *testing.T) {
	name := "target-deploy"
	ns := "default"

	// Create target deployment
	deploy := &appsv1.Deployment{}
	deploy.ObjectMeta.Name = name
	deploy.ObjectMeta.Namespace = ns
	deploy.Spec.Template.Labels = make(map[string]string)
	deploy.Spec.Template.Annotations = make(map[string]string)

	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	typedClient := fake.NewSimpleClientset(deploy)

	// Create dynamic client (for the patcher).
	// Note: typed and dynamic fake clients have separate object stores.
	// Patches applied via the dynamic client are verified by reading back
	// from the dynamic client. The typed client is only used for the
	// idempotency check inside patchDeployment.
	scheme := runtime.NewScheme()
	appsv1.AddToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			deployGVR: "DeploymentList",
		},
		deploy,
	)

	// Create patcher
	wp := workloadpatcher.NewPatcher(dynamicClient, func() bool { return true })
	p := patcher{
		k8sClient:          typedClient,
		patchClient:        wp,
		isLeader:           func() bool { return true },
		telemetryCollector: telemetry.NewNoopCollector(),
	}

	// Helpers to read back from the dynamic client
	getDeploy := func() *unstructured.Unstructured {
		obj, err := dynamicClient.Resource(deployGVR).Namespace(ns).Get(context.TODO(), name, metav1.GetOptions{})
		require.NoError(t, err)
		return obj
	}
	getAnnotations := func(obj *unstructured.Unstructured) map[string]string {
		annots, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "annotations")
		return annots
	}
	getTemplateAnnotations := func(obj *unstructured.Unstructured) map[string]string {
		annots, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
		return annots
	}
	getTemplateLabels := func(obj *unstructured.Unstructured) map[string]string {
		labels, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
		return labels
	}

	// Create request skeleton
	req := Request{
		ID:        "id",
		K8sTarget: K8sTarget{Kind: KindDeployment, Namespace: ns, Name: name},
		LibConfig: common.LibConfig{Language: "java", Version: "latest"},
	}

	// Stage the patch
	req.Action = StageConfig
	req.Revision = 12
	require.NoError(t, p.patchDeployment(req))

	// Check: stage should only set deployment-level annotations, not template
	got := getDeploy()
	require.NotContains(t, getTemplateLabels(got), "admission.datadoghq.com/enabled")
	require.NotContains(t, getTemplateAnnotations(got), "admission.datadoghq.com/java-lib.version")
	require.NotContains(t, getTemplateAnnotations(got), "admission.datadoghq.com/java-lib.config.v1")
	require.NotContains(t, getTemplateAnnotations(got), "admission.datadoghq.com/rc.id")
	require.NotContains(t, getTemplateAnnotations(got), "admission.datadoghq.com/rc.rev")
	require.Equal(t, "id", getAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "12", getAnnotations(got)["admission.datadoghq.com/rc.rev"])

	// Apply the patch (enable)
	req.Action = EnableConfig
	req.Revision = 123
	require.NoError(t, p.patchDeployment(req))

	// Check: enable should set template labels, annotations, and metadata annotations
	got = getDeploy()
	require.Equal(t, "true", getTemplateLabels(got)["admission.datadoghq.com/enabled"])
	require.Equal(t, "latest", getTemplateAnnotations(got)["admission.datadoghq.com/java-lib.version"])
	require.Equal(t, `{"library_language":"java","library_version":"latest"}`, getTemplateAnnotations(got)["admission.datadoghq.com/java-lib.config.v1"])
	require.Equal(t, "id", getTemplateAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "123", getTemplateAnnotations(got)["admission.datadoghq.com/rc.rev"])
	require.Equal(t, "id", getAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "123", getAnnotations(got)["admission.datadoghq.com/rc.rev"])

	// Patch again to disable lib injection (aka revert)
	req.Action = DisableConfig
	req.Revision = 1234
	require.NoError(t, p.patchDeployment(req))

	// Check: disable should set enabled=false, remove lib annotations, keep rc tracking
	got = getDeploy()
	require.Equal(t, "false", getTemplateLabels(got)["admission.datadoghq.com/enabled"])
	require.NotContains(t, getTemplateAnnotations(got), "admission.datadoghq.com/java-lib.version")
	require.NotContains(t, getTemplateAnnotations(got), "admission.datadoghq.com/java-lib.config.v1")
	require.Equal(t, "id", getTemplateAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "1234", getTemplateAnnotations(got)["admission.datadoghq.com/rc.rev"])
	require.Equal(t, "id", getAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "1234", getAnnotations(got)["admission.datadoghq.com/rc.rev"])

	// Apply a new patch with a new config (tracing tags)
	req.Action = EnableConfig
	req.Revision = 12345
	req.LibConfig = common.LibConfig{Language: "java", Version: "latest", TracingTags: []string{"k1:v1", "k2:v2"}}
	require.NoError(t, p.patchDeployment(req))

	// Check: re-enable with tracing tags
	got = getDeploy()
	require.Equal(t, "true", getTemplateLabels(got)["admission.datadoghq.com/enabled"])
	require.Equal(t, "latest", getTemplateAnnotations(got)["admission.datadoghq.com/java-lib.version"])
	require.Equal(t, `{"library_language":"java","library_version":"latest","tracing_tags":["k1:v1","k2:v2"]}`, getTemplateAnnotations(got)["admission.datadoghq.com/java-lib.config.v1"])
	require.Equal(t, "id", getTemplateAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "12345", getTemplateAnnotations(got)["admission.datadoghq.com/rc.rev"])
	require.Equal(t, "id", getAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "12345", getAnnotations(got)["admission.datadoghq.com/rc.rev"])

	// Stage a new patch with a new config
	req.Action = StageConfig
	req.Revision = 123456
	req.LibConfig = common.LibConfig{Language: "java", Version: "v1", TracingTags: []string{"foo:bar"}}
	require.NoError(t, p.patchDeployment(req))

	// Check: only deployment annotations should be updated, template unchanged
	got = getDeploy()
	require.Equal(t, "true", getTemplateLabels(got)["admission.datadoghq.com/enabled"])
	require.Equal(t, "latest", getTemplateAnnotations(got)["admission.datadoghq.com/java-lib.version"])
	require.Equal(t, `{"library_language":"java","library_version":"latest","tracing_tags":["k1:v1","k2:v2"]}`, getTemplateAnnotations(got)["admission.datadoghq.com/java-lib.config.v1"])
	require.Equal(t, "id", getTemplateAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "12345", getTemplateAnnotations(got)["admission.datadoghq.com/rc.rev"])
	require.Equal(t, "id", getAnnotations(got)["admission.datadoghq.com/rc.id"])
	require.Equal(t, "123456", getAnnotations(got)["admission.datadoghq.com/rc.rev"])
}
