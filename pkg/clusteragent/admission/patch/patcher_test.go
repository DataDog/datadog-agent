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

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPatchDeployment(t *testing.T) {
	client := fake.NewSimpleClientset()
	name := "target-deploy"
	ns := "default"

	// Create target deployment
	deploy := appsv1.Deployment{}
	deploy.ObjectMeta.Name = name
	deploy.ObjectMeta.Namespace = ns
	deploy.Spec.Template.Labels = make(map[string]string)
	deploy.Spec.Template.Annotations = make(map[string]string)
	client.AppsV1().Deployments(ns).Create(context.TODO(), &deploy, metav1.CreateOptions{})

	// Create patcher
	p := patcher{
		k8sClient: client,
		isLeader:  func() bool { return true },
	}

	// Create request skeleton
	req := PatchRequest{
		ID:        "id",
		K8sTarget: K8sTarget{Kind: KindDeployment, Namespace: ns, Name: name},
		LibConfig: common.LibConfig{Language: "java", Version: "latest"},
	}

	// Stage the patch
	req.Action = StageConfig
	req.Revision = 12
	require.NoError(t, p.patchDeployment(req))

	// Check the patch
	got, err := client.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, got.Spec.Template.Labels, "admission.datadoghq.com/enabled")
	require.NotContains(t, got.Spec.Template.Annotations, "admission.datadoghq.com/java-lib.version")
	require.NotContains(t, got.Spec.Template.Annotations, "admission.datadoghq.com/java-lib.config.v1")
	require.NotContains(t, got.Spec.Template.Annotations, "admission.datadoghq.com/rc.id")
	require.NotContains(t, got.Spec.Template.Annotations, "admission.datadoghq.com/rc.rev")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.rev"], "12")

	// Apply the patch
	req.Action = EnableConfig
	req.Revision = 123
	require.NoError(t, p.patchDeployment(req))

	// Check the patch
	got, err = client.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.Spec.Template.Labels["admission.datadoghq.com/enabled"], "true")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/java-lib.version"], "latest")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/java-lib.config.v1"], `{"library_language":"java","library_version":"latest"}`)
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.rev"], "123")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.rev"], "123")

	// Patch again to disable lib injection (aka revert)
	req.Action = DisableConfig
	req.Revision = 1234
	require.NoError(t, p.patchDeployment(req))

	// Check the new patch
	got, err = client.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.Spec.Template.Labels["admission.datadoghq.com/enabled"], "false")
	require.NotContains(t, got.Spec.Template.Annotations, "admission.datadoghq.com/java-lib.version")
	require.NotContains(t, got.Spec.Template.Annotations, "admission.datadoghq.com/java-lib.config.v1")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.rev"], "1234")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.rev"], "1234")

	// Apply a new patch with a new config
	req.Action = EnableConfig
	req.Revision = 12345
	req.LibConfig = common.LibConfig{Language: "java", Version: "latest", TracingTags: []string{"k1:v1", "k2:v2"}}
	require.NoError(t, p.patchDeployment(req))

	// Check the new patch
	got, err = client.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.Spec.Template.Labels["admission.datadoghq.com/enabled"], "true")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/java-lib.version"], "latest")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/java-lib.config.v1"], `{"library_language":"java","library_version":"latest","tracing_tags":["k1:v1","k2:v2"]}`)
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.rev"], "12345")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.rev"], "12345")

	// Stage a new patch with a new config
	req.Action = StageConfig
	req.Revision = 123456
	req.LibConfig = common.LibConfig{Language: "java", Version: "v1", TracingTags: []string{"foo:bar"}}
	require.NoError(t, p.patchDeployment(req))

	// Check the new patch, only the deployment annotations should be updated
	got, err = client.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.Spec.Template.Labels["admission.datadoghq.com/enabled"], "true")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/java-lib.version"], "latest")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/java-lib.config.v1"], `{"library_language":"java","library_version":"latest","tracing_tags":["k1:v1","k2:v2"]}`)
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Spec.Template.Annotations["admission.datadoghq.com/rc.rev"], "12345")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.id"], "id")
	require.Equal(t, got.Annotations["admission.datadoghq.com/rc.rev"], "123456")
}
