// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func newTestHPA(name, namespace string, maxReplicas int32, annotations map[string]string) autoscalingv2.HorizontalPodAutoscaler {
	minReplicas := int32(1)
	return autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "my-deploy",
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
		},
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestNewHPAWebhook(t *testing.T) {
	cfg := mutatecommon.FakeConfigWithValues(t, map[string]any{
		"autoscaling.workload.enabled": true,
	})
	w := NewHPAWebhook(cfg)

	assert.Equal(t, hpaWebhookName, w.Name())
	assert.Equal(t, hpaWebhookEndpoint, w.Endpoint())
	assert.True(t, w.IsEnabled())
	assert.Equal(t, admcommon.WebhookType(admcommon.MutatingWebhook), w.WebhookType())
	assert.Equal(t, []admissionregistrationv1.OperationType{admissionregistrationv1.Update}, w.Operations())
	assert.Equal(t, map[string][]string{"autoscaling": {"horizontalpodautoscalers"}}, w.Resources())
}

func TestHPAWebhook_MatchConditions(t *testing.T) {
	cfg := mutatecommon.FakeConfigWithValues(t, map[string]any{})
	w := NewHPAWebhook(cfg)
	conditions := w.MatchConditions()
	require.Len(t, conditions, 1)
	assert.Equal(t, "managed-by-dpa", conditions[0].Name)
	assert.Contains(t, conditions[0].Expression, model.HPAManagedByDPAAnnotation)
}

func TestHPAWebhook_revertHPASpec_managed(t *testing.T) {
	// Old (disabled) HPA with maxReplicas=1000 (the sentinel value set by the migration).
	oldHPA := newTestHPA("my-hpa", "default", 1000, map[string]string{
		model.HPAManagedByDPAAnnotation: "default/my-dpa",
	})
	// Incoming HPA where someone tried to change maxReplicas back to 5.
	incomingHPA := newTestHPA("my-hpa", "default", 5, map[string]string{
		model.HPAManagedByDPAAnnotation: "default/my-dpa",
	})

	w := &HPAWebhook{}
	req := &admission.Request{
		Name:      "my-hpa",
		Namespace: "default",
		Object:    mustMarshal(t, incomingHPA),
		OldObject: mustMarshal(t, oldHPA),
	}

	resp := w.revertHPASpec(req)

	require.NotNil(t, resp)
	assert.True(t, resp.Allowed)
	require.NotNil(t, resp.Warnings)
	assert.Len(t, resp.Warnings, 1)
	assert.Equal(t,
		"HPA default/my-hpa is managed by DatadogPodAutoscaler default/my-dpa and cannot be modified directly. "+
			"Your change has been reverted. If you no longer need the HPA, you can safely delete it.",
		resp.Warnings[0],
	)

	// Verify the patch reverts the spec to oldHPA.Spec.
	require.NotNil(t, resp.Patch)
	var ops []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Patch, &ops))
	require.Len(t, ops, 1)
	assert.Equal(t, `"replace"`, string(ops[0]["op"]))
	assert.Equal(t, `"/spec"`, string(ops[0]["path"]))

	// The patch value must match the old spec (maxReplicas=1000).
	var patchedSpec autoscalingv2.HorizontalPodAutoscalerSpec
	require.NoError(t, json.Unmarshal(ops[0]["value"], &patchedSpec))
	assert.Equal(t, int32(1000), patchedSpec.MaxReplicas)
}

func TestHPAWebhook_revertHPASpec_not_managed(t *testing.T) {
	// HPA without the DPA annotation → webhook should be a no-op.
	hpa := newTestHPA("my-hpa", "default", 5, nil)

	w := &HPAWebhook{}
	req := &admission.Request{
		Name:      "my-hpa",
		Namespace: "default",
		Object:    mustMarshal(t, hpa),
	}

	resp := w.revertHPASpec(req)

	require.NotNil(t, resp)
	assert.True(t, resp.Allowed)
	assert.Empty(t, resp.Patch)
	assert.Empty(t, resp.Warnings)
}

func TestHPAWebhook_revertHPASpec_invalid_object(t *testing.T) {
	w := &HPAWebhook{}
	req := &admission.Request{
		Name:      "my-hpa",
		Namespace: "default",
		Object:    []byte("not-json"),
	}

	resp := w.revertHPASpec(req)

	require.NotNil(t, resp)
	assert.True(t, resp.Allowed)
	assert.Empty(t, resp.Patch)
}

func TestHPAWebhook_revertHPASpec_invalid_old_object(t *testing.T) {
	incomingHPA := newTestHPA("my-hpa", "default", 5, map[string]string{
		model.HPAManagedByDPAAnnotation: "default/my-dpa",
	})

	w := &HPAWebhook{}
	req := &admission.Request{
		Name:      "my-hpa",
		Namespace: "default",
		Object:    mustMarshal(t, incomingHPA),
		OldObject: []byte("not-json"),
	}

	resp := w.revertHPASpec(req)

	require.NotNil(t, resp)
	assert.True(t, resp.Allowed)
	assert.Empty(t, resp.Patch)
}

func TestHPAWebhook_revertHPASpec_preserves_metrics(t *testing.T) {
	cpuTarget := int32(50)
	oldHPA := newTestHPA("my-hpa", "default", 1000, map[string]string{
		model.HPAManagedByDPAAnnotation: "default/my-dpa",
	})
	oldHPA.Spec.Metrics = []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: "cpu",
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &cpuTarget,
				},
			},
		},
	}
	// Incoming HPA where the metrics were removed.
	incomingHPA := newTestHPA("my-hpa", "default", 5, map[string]string{
		model.HPAManagedByDPAAnnotation: "default/my-dpa",
	})

	w := &HPAWebhook{}
	req := &admission.Request{
		Name:      "my-hpa",
		Namespace: "default",
		Object:    mustMarshal(t, incomingHPA),
		OldObject: mustMarshal(t, oldHPA),
	}

	resp := w.revertHPASpec(req)

	require.NotNil(t, resp)
	assert.True(t, resp.Allowed)

	var ops []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Patch, &ops))
	require.Len(t, ops, 1)

	var patchedSpec autoscalingv2.HorizontalPodAutoscalerSpec
	require.NoError(t, json.Unmarshal(ops[0]["value"], &patchedSpec))
	require.Len(t, patchedSpec.Metrics, 1)
	require.NotNil(t, patchedSpec.Metrics[0].Resource)
	assert.Equal(t, int32(50), *patchedSpec.Metrics[0].Resource.Target.AverageUtilization)
}

func TestHPAWebhook_WebhookFunc(t *testing.T) {
	oldHPA := newTestHPA("my-hpa", "ns", 1000, map[string]string{
		model.HPAManagedByDPAAnnotation: "ns/my-dpa",
	})
	incomingHPA := newTestHPA("my-hpa", "ns", 3, map[string]string{
		model.HPAManagedByDPAAnnotation: "ns/my-dpa",
	})

	cfg := mutatecommon.FakeConfigWithValues(t, map[string]any{
		"autoscaling.workload.enabled": true,
	})
	w := NewHPAWebhook(cfg)

	fn := w.WebhookFunc()
	resp := fn(&admission.Request{
		Name:      "my-hpa",
		Namespace: "ns",
		Object:    mustMarshal(t, incomingHPA),
		OldObject: mustMarshal(t, oldHPA),
	})

	require.NotNil(t, resp)
	assert.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patch)

}
