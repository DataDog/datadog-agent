// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package probe

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const testNamespace = "test-probe-ns"

func newTestProbe(client *fakeclientset.Clientset) *Probe {
	return &Probe{
		k8sClient:  client,
		namespace:  testNamespace,
		logLimiter: log.NewLogLimit(10, time.Minute),
	}
}

func webhookReachableReactor(action k8stesting.Action) (bool, runtime.Object, error) {
	createAction := action.(k8stesting.CreateAction)
	pod := createAction.GetObject().(*corev1.Pod)
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[admcommon.ProbeReceivedAnnotationKey] = "true"
	return true, pod, nil
}

func TestExecute_WebhookReachable(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", webhookReachableReactor)

	p := newTestProbe(client)
	err := p.execute(context.Background())
	assert.NoError(t, err)
}

func TestExecute_WebhookNotReachable(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	p := newTestProbe(client)

	err := p.execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errProbeNotReceived)
}

func TestExecute_PodHasCorrectLabelsAndNamespace(t *testing.T) {
	var createdPod *corev1.Pod
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		createdPod = createAction.GetObject().(*corev1.Pod)
		if createdPod.Annotations == nil {
			createdPod.Annotations = make(map[string]string)
		}
		createdPod.Annotations[admcommon.ProbeReceivedAnnotationKey] = "true"
		return true, createdPod, nil
	})

	p := newTestProbe(client)
	err := p.execute(context.Background())
	require.NoError(t, err)
	require.NotNil(t, createdPod)
	assert.Equal(t, "true", createdPod.Labels[admcommon.EnabledLabelKey])
	assert.Equal(t, "true", createdPod.Labels[admcommon.ProbeLabelKey])
	assert.Equal(t, testNamespace, createdPod.Namespace)
	assert.NotEmpty(t, createdPod.GenerateName)
}

func TestExecute_UsesDryRun(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		pod := createAction.GetObject().(*corev1.Pod)
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[admcommon.ProbeReceivedAnnotationKey] = "true"
		assert.Contains(t, action.(k8stesting.CreateActionImpl).CreateOptions.DryRun, metav1.DryRunAll)
		return true, pod, nil
	})

	p := newTestProbe(client)
	err := p.execute(context.Background())
	assert.NoError(t, err)
}

func TestExecute_NamespaceNotFound(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, testNamespace)
	})

	p := newTestProbe(client)
	err := p.execute(context.Background())
	require.Error(t, err)
	assert.True(t, k8serrors.IsNotFound(err))
}

func TestExecute_Forbidden(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", fmt.Errorf("forbidden"))
	})

	p := newTestProbe(client)
	err := p.execute(context.Background())
	require.Error(t, err)
	assert.True(t, k8serrors.IsForbidden(err))
}

func TestRunProbe_StatsOnSuccess(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", webhookReachableReactor)

	p := newTestProbe(client)
	p.runProbe(context.Background())

	snap := p.GetStatsSnapshot()
	assert.Equal(t, int64(1), snap.TotalExecutions)
	assert.Equal(t, int64(1), snap.SuccessCount)
	assert.Equal(t, int64(0), snap.FailCount)
	assert.True(t, snap.LastExecutionSuccess)
	assert.Empty(t, snap.LastExecutionError)
	assert.False(t, snap.LastSuccessTime.IsZero())
	assert.Empty(t, snap.ConfigError)
}

func TestRunProbe_StatsOnConnectivityFailure(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	p := newTestProbe(client)

	p.runProbe(context.Background())

	snap := p.GetStatsSnapshot()
	assert.Equal(t, int64(1), snap.TotalExecutions)
	assert.Equal(t, int64(0), snap.SuccessCount)
	assert.Equal(t, int64(1), snap.FailCount)
	assert.False(t, snap.LastExecutionSuccess)
	assert.Contains(t, snap.LastExecutionError, "not annotated")
	assert.True(t, snap.LastSuccessTime.IsZero())
	assert.Empty(t, snap.ConfigError)
}

func TestRunProbe_StatsOnNamespaceNotFound(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, testNamespace)
	})

	p := newTestProbe(client)
	p.runProbe(context.Background())

	snap := p.GetStatsSnapshot()
	assert.Equal(t, int64(1), snap.TotalExecutions)
	assert.Equal(t, int64(1), snap.FailCount)
	assert.Contains(t, snap.ConfigError, "does not exist")
}

func TestRunProbe_StatsOnForbidden(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", fmt.Errorf("forbidden"))
	})

	p := newTestProbe(client)
	p.runProbe(context.Background())

	snap := p.GetStatsSnapshot()
	assert.Equal(t, int64(1), snap.TotalExecutions)
	assert.Equal(t, int64(1), snap.FailCount)
	assert.Contains(t, snap.ConfigError, "does not have permission")
}

func TestRunProbe_ConfigErrorClearedOnSuccess(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", fmt.Errorf("forbidden"))
	})

	p := newTestProbe(client)
	p.runProbe(context.Background())
	assert.NotEmpty(t, p.GetStatsSnapshot().ConfigError)

	client.ReactionChain = nil
	client.PrependReactor("create", "pods", webhookReachableReactor)

	p.runProbe(context.Background())
	snap := p.GetStatsSnapshot()
	assert.Empty(t, snap.ConfigError)
	assert.Equal(t, int64(1), snap.SuccessCount)
}

func TestGetStatsForStatus_SuccessRate(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", webhookReachableReactor)

	p := newTestProbe(client)
	p.runProbe(context.Background())
	p.runProbe(context.Background())

	client.ReactionChain = nil
	p.runProbe(context.Background())

	status := p.GetStatsForStatus()
	assert.Equal(t, "66.7%", status["SuccessRate"])
	assert.Equal(t, int64(3), status["TotalExecutions"])
	assert.Equal(t, int64(2), status["SuccessCount"])
	assert.Equal(t, int64(1), status["FailCount"])
	assert.Equal(t, testNamespace, status["Namespace"])
}

func TestGetStatsForStatus_ConfigError(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, testNamespace)
	})

	p := newTestProbe(client)
	p.runProbe(context.Background())

	status := p.GetStatsForStatus()
	assert.Equal(t, testNamespace, status["Namespace"])
	assert.Contains(t, status["ConfigError"], "does not exist")
	_, hasTotalExecutions := status["TotalExecutions"]
	assert.False(t, hasTotalExecutions)
}

func TestDiagnosticHintForProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		containsStr string
	}{
		{name: "EKS mentions security groups", provider: "eks", containsStr: "security groups"},
		{name: "GKE mentions firewall", provider: "gke", containsStr: "firewall"},
		{name: "AKS mentions providers.aks.enabled", provider: "aks", containsStr: "providers.aks.enabled"},
		{name: "unknown provider gives generic hint", provider: "", containsStr: "port 8000"},
		{name: "other provider gives generic hint", provider: "other", containsStr: "port 8000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := diagnosticHintForProvider(tt.provider)
			assert.Contains(t, hint, tt.containsStr)
		})
	}
}
