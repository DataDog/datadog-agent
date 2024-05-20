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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPatchNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	name := "target-namespace"
	env := "dev"

	// Create target namespace
	ns := v1.Namespace{}
	ns.ObjectMeta.Name = name
	ns.ObjectMeta.Labels = make(map[string]string)
	ns.ObjectMeta.Annotations = make(map[string]string)
	client.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})

	// Create patcher
	p := patcher{
		k8sClient:            client,
		isLeader:             func() bool { return true },
		telemetryCollector:   telemetry.NewNoopCollector(),
		configIDToNamespaces: make(map[string]*[]string),
	}

	// Create request skeleton
	req := Request{
		ID: "id",
		K8sTarget: &K8sTarget{
			ClusterTargets: []K8sClusterTarget{
				{ClusterName: "test-cluster", Enabled: truePtr(), EnabledNamespaces: &[]string{name}},
			},
		},
		LibConfig: common.LibConfig{Env: &env},
	}

	// Enable the configuration
	req.ID = "12345"
	req.Action = EnableConfig
	req.Revision = 12
	require.NoError(t, p.patchNamespaces(req))

	// Check the patch
	got, err := client.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.ObjectMeta.Labels[k8sutil.RcLabelKey], "true")
	require.Equal(t, got.ObjectMeta.Annotations[k8sutil.RcIDAnnotKey], "12345")
	require.Equal(t, got.ObjectMeta.Annotations[k8sutil.RcRevisionAnnotKey], "12")
	require.Equal(t, got.Labels[k8sutil.RcLabelKey], "true")
	require.Equal(t, got.Annotations[k8sutil.RcIDAnnotKey], "12345")
	require.Equal(t, got.Annotations[k8sutil.RcRevisionAnnotKey], "12")

	// Enable the configuration on the same namespace
	req.ID = "123456"
	req.Action = EnableConfig
	req.Revision = 123
	require.NoError(t, p.patchNamespaces(req))

	// Check the patch
	got, err = client.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.ObjectMeta.Labels[k8sutil.RcLabelKey], "true")
	require.Equal(t, got.ObjectMeta.Annotations[k8sutil.RcIDAnnotKey], "12345")
	require.Equal(t, got.ObjectMeta.Annotations[k8sutil.RcRevisionAnnotKey], "12")
	require.Equal(t, got.Labels[k8sutil.RcLabelKey], "true")
	require.Equal(t, got.Annotations[k8sutil.RcIDAnnotKey], "12345")
	require.Equal(t, got.Annotations[k8sutil.RcRevisionAnnotKey], "12")

	// Disable the configuration
	req.ID = "12345"
	req.Action = DisableConfig
	req.Revision = 13
	require.NoError(t, p.patchNamespaces(req))

	// Check the patch
	got, err = client.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, got.ObjectMeta.Labels[k8sutil.RcLabelKey], "false")
	require.Equal(t, got.ObjectMeta.Annotations[k8sutil.RcIDAnnotKey], "12345")
	require.Equal(t, got.ObjectMeta.Annotations[k8sutil.RcRevisionAnnotKey], "12")
	require.Equal(t, got.Labels[k8sutil.RcLabelKey], "false")
	require.Equal(t, got.Annotations[k8sutil.RcIDAnnotKey], "12345")
	require.Equal(t, got.Annotations[k8sutil.RcRevisionAnnotKey], "12")

	// Delete configuration
	req.Action = DeleteConfig
	req.Revision = 15
	require.NoError(t, p.patchNamespaces(req))

	// Check the patch
	got, err = client.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, got.ObjectMeta.Labels, k8sutil.RcLabelKey)
	require.NotContains(t, got.ObjectMeta.Annotations, k8sutil.RcIDAnnotKey)
	require.NotContains(t, got.ObjectMeta.Annotations, k8sutil.RcRevisionAnnotKey)
	require.NotContains(t, got.Labels, k8sutil.RcLabelKey)
	require.NotContains(t, got.Annotations, k8sutil.RcIDAnnotKey)
	require.NotContains(t, got.Annotations, k8sutil.RcRevisionAnnotKey)
}
