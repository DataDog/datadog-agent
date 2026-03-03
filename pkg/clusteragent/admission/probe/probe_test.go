// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package probe

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestExecute_WebhookReachable(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		pod := createAction.GetObject().(*corev1.Pod)
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[admcommon.ProbeReceivedAnnotationKey] = "true"
		return true, pod, nil
	})

	p := &Probe{
		k8sClient:  client,
		logLimiter: log.NewLogLimit(1, 10*time.Minute),
	}

	err := p.execute(context.Background())
	assert.NoError(t, err)
}

func TestExecute_WebhookNotReachable(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()

	p := &Probe{
		k8sClient:  client,
		logLimiter: log.NewLogLimit(1, 10*time.Minute),
	}

	err := p.execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errProbeNotReceived)
}

func TestExecute_PodHasCorrectLabels(t *testing.T) {
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

	p := &Probe{
		k8sClient:  client,
		logLimiter: log.NewLogLimit(1, 10*time.Minute),
	}

	err := p.execute(context.Background())
	require.NoError(t, err)
	require.NotNil(t, createdPod)
	assert.Equal(t, "true", createdPod.Labels[admcommon.EnabledLabelKey])
	assert.Equal(t, "true", createdPod.Labels[admcommon.ProbeLabelKey])
	assert.Equal(t, probeNamespace, createdPod.Namespace)

	opts := createdPod.GetObjectMeta().(*metav1.ObjectMeta)
	assert.NotEmpty(t, opts.GenerateName)
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

	p := &Probe{
		k8sClient:  client,
		logLimiter: log.NewLogLimit(1, 10*time.Minute),
	}

	err := p.execute(context.Background())
	assert.NoError(t, err)
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
