// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	clusterspot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
)

type countingHandler struct{ created, deleted int }

func (h *countingHandler) PlanPlacement(*corev1.Pod) (*clusterspot.PodPlacement, error) {
	h.created++
	return &clusterspot.PodPlacement{
		NodeSelector: map[string]string{"nodeType": "spot"},
		Labels:       map[string]string{"placement": "spot"},
		Tolerations:  []corev1.Toleration{{Key: "spot", Operator: corev1.TolerationOpExists}},
	}, nil
}
func (h *countingHandler) PodDeleted(*corev1.Pod) { h.deleted++ }

// Scheduler tests cover the real placement and tracker policy. This boundary
// test verifies that admission simulation never invokes that decision twice.
func TestAdmissionInvokesPlacementOnce(t *testing.T) {
	raw := []byte(`{"metadata":{"name":"app"},"spec":{"containers":[{"name":"app"}],"tolerations":[{"key":"spot","operator":"Exists","custom":18446744073709551617},{"key":"spot","operator":"Exists","custom":0.123456789012345678901}]}}`)
	for _, operation := range []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Delete} {
		for _, dryRun := range []bool{false, true} {
			t.Run(string(operation)+map[bool]string{false: "/admission", true: "/dry run"}[dryRun], func(t *testing.T) {
				handler := &countingHandler{}
				webhook := NewWebhook(coreconfig.NewMock(t), handler)
				response := webhook.WebhookFunc()(&admission.Request{Object: raw, OldObject: raw, Namespace: "test-ns", Operation: operation, DryRun: &dryRun})
				require.True(t, response.Allowed)
				if dryRun {
					require.Zero(t, handler.created)
					require.Zero(t, handler.deleted)
					require.Empty(t, response.Patch)
				} else if operation == admissionregistrationv1.Delete {
					require.Zero(t, handler.created)
					require.Equal(t, 1, handler.deleted)
					require.Empty(t, response.Patch)
				} else {
					require.Equal(t, 1, handler.created)
					require.Zero(t, handler.deleted)
					p, err := jsonpatch.DecodePatch(response.Patch)
					require.NoError(t, err)
					options := jsonpatch.NewApplyOptions()
					options.SupportNegativeIndices = false
					options.AllowMissingPathOnRemove = false
					options.EnsurePathExistsOnAdd = false
					out, err := p.ApplyWithOptions(raw, options)
					require.NoError(t, err)
					require.Contains(t, string(out), `18446744073709551617`)
					require.Contains(t, string(out), `0.123456789012345678901`)
					var pod corev1.Pod
					require.NoError(t, json.Unmarshal(out, &pod))
					require.Len(t, pod.Spec.Tolerations, 3)
					require.Equal(t, "spot", pod.Spec.NodeSelector["nodeType"])
				}
			})
		}
	}
}
