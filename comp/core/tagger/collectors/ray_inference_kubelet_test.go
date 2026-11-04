// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package collectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestRayInferenceTagsFromKubeletSpec(t *testing.T) {
	for _, primaryImage := range []string{"rayproject/ray-llm:2.56.0-py312-cu130", "rayproject/ray:latest"} {
		for _, pending := range []bool{false, true} {
			name := "running"
			if pending {
				name = "pending"
			}
			t.Run(primaryImage+"/"+name, func(t *testing.T) {
				store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
					core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				))
				pod := &kubelet.Pod{
					Metadata: kubelet.PodMetadata{Name: "ray-worker", UID: "ray-pod", Labels: map[string]string{
						"ray.io/cluster": "nlq-mixed-nqlnx", "ray.io/serve": "false",
						"ray.io/node-type": "worker", "ray.io/group": "gpu-group",
					}},
					Spec: kubelet.Spec{Containers: []kubelet.ContainerSpec{
						{Name: "llm", Image: primaryImage},
						{Name: "sidecar", Image: "vllm/vllm-openai:latest"},
					}},
					// Status order can differ from spec order, and the Ray container may not have started.
					Status: kubelet.Status{Containers: []kubelet.ContainerStatus{
						{Name: "sidecar", ID: "containerd://sidecar", Image: "vllm/vllm-openai:latest"},
					}},
				}
				if !pending {
					pod.Status.Containers = append(pod.Status.Containers, kubelet.ContainerStatus{Name: "llm", ID: "containerd://ray", Image: primaryImage})
				}
				var parsed *workloadmeta.KubernetesPod
				for _, event := range util.ParseKubeletPods([]*kubelet.Pod{pod}, false, store) {
					if entity, ok := event.Entity.(*workloadmeta.KubernetesPod); ok {
						parsed = entity
					}
				}
				require.NotNil(t, parsed)
				require.Len(t, parsed.Containers, 2)
				assert.Equal(t, "llm", parsed.Containers[0].Name)
				assert.Equal(t, "sidecar", parsed.Containers[1].Name)
				if pending {
					assert.Empty(t, parsed.Containers[0].ID)
				} else {
					assert.Equal(t, "ray", parsed.Containers[0].ID)
				}
				extractor := NewPodTagExtractor(config.NewMock(t), store)
				tags := extractor.Extract(parsed, types.LowCardinality)
				if primaryImage == "rayproject/ray-llm:2.56.0-py312-cu130" {
					assert.Contains(t, tags, "inference_workload:nlq-mixed")
					assert.Contains(t, tags, "inference_workload_component:gpu-group")
				} else {
					assert.NotContains(t, tags, "inference_workload:nlq-mixed")
					assert.NotContains(t, tags, "inference_workload_component:gpu-group")
				}
			})
		}
	}
}
