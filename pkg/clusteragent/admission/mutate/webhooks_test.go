// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package mutate_test

import (
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/spot"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
)

type spotHandler func(*corev1.Pod) (bool, error)

func (h spotHandler) PodCreated(p *corev1.Pod) (bool, error) { return h(p) }
func (spotHandler) PodDeleted(*corev1.Pod)                   {}

type recommendationPatcher struct {
	workload.PodPatcher // Only ApplyRecommendations is called by admission.
	mutate              func(*corev1.Pod) (bool, error)
}

func (p recommendationPatcher) ApplyRecommendations(pod *corev1.Pod) (bool, error) {
	return p.mutate(pod)
}

// Exercise the real admission wrappers with representative mutation results.
// Apply each returned patch to raw JSON so unknown fields remain observable.
func TestWebhookPatchesPreserveUnknownFields(t *testing.T) {
	const raw = `{"metadata":{"name":"app"},"spec":{"containers":[{"name":"app","image":"app:v1","securityContext":{"capabilities":{"ambient":["CHOWN"]}}}],"tolerations":[{"key":"existing","operator":"Exists","futureField":"toleration"}],"volumes":[{"name":"v","emptyDir":{},"futureField":"volume"}]}}`
	for _, kind := range []string{"autoscaling", "spot", "library injection"} {
		for _, changed := range []bool{false, true} {
			name := kind + "/no-op"
			if changed {
				name = kind + "/mutated"
			}
			t.Run(name, func(t *testing.T) {
				mutator := func(p *corev1.Pod) (bool, error) {
					if !changed {
						return false, nil
					}
					switch kind {
					case "autoscaling":
						p.Spec.Containers[0].Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}
					case "spot":
						p.Spec.Tolerations = append(p.Spec.Tolerations, corev1.Toleration{Key: "spot", Operator: corev1.TolerationOpExists})
					case "library injection":
						common.AddAnnotation(p, "injection-status", "blocked")
						return false, nil // Blocked policies still write annotations.
					}
					return true, nil
				}
				cfg := config.NewMock(t)
				var hook admission.WebhookFunc
				switch kind {
				case "autoscaling":
					hook = autoscaling.NewWebhook(recommendationPatcher{mutate: mutator}, cfg).WebhookFunc()
				case "spot":
					hook = spot.NewWebhook(cfg, spotHandler(mutator)).WebhookFunc()
				case "library injection":
					m := common.MutatorFunc(func(p *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) { return mutator(p) })
					w, err := autoinstrumentation.NewWebhook(autoinstrumentation.NewWebhookConfig(cfg), m, &autoinstrumentation.LabelSelectors{})
					require.NoError(t, err)
					hook = w.WebhookFunc()
				}
				response := hook(&admission.Request{Object: []byte(raw), Namespace: "default", Operation: admissionregistrationv1.Create})
				require.True(t, response.Allowed)
				require.Nil(t, response.Result)
				patch, err := jsonpatch.DecodePatch(response.Patch)
				require.NoError(t, err)
				actual, err := patch.Apply([]byte(raw))
				require.NoError(t, err)
				var expected map[string]any
				require.NoError(t, json.Unmarshal([]byte(raw), &expected))
				if changed {
					spec := expected["spec"].(map[string]any)
					switch kind {
					case "autoscaling":
						spec["containers"].([]any)[0].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"cpu": "100m"}}
					case "spot":
						spec["tolerations"] = append(spec["tolerations"].([]any), map[string]any{"key": "spot", "operator": "Exists"})
					case "library injection":
						expected["metadata"].(map[string]any)["annotations"] = map[string]any{"injection-status": "blocked"}
					}
				} else {
					require.Empty(t, patch)
				}
				want, err := json.Marshal(expected)
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(actual))
			})
		}
	}
}
