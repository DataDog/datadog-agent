// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package summary

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
)

var (
	//entity id: [fake tags]
	entityTags = map[string][]string{
		//pod name in summary: datadog-agent-linux-hn9f2
		"kubernetes_pod_uid://foobar": {
			"app:datadog-agent",
		},
		"kubernetes_pod_uid://d2dfd16e-e829-4f66-91d5-f9233ca7332b": {
			"app:pending-pod",
		},
		"container_id://agent-01": {
			"kube_namespace:datadog-agent-helm",
			"pod_name:datadog-agent-linux-hn9f2",
			"kube_container_name:agent",
		},
		"container_id://agent-02": {
			"kube_namespace:datadog-agent-helm",
			"pod_name:datadog-agent-linux-hn9f2",
			"kube_container_name:process-agent",
		},
		"container_id://agent-03": {
			"kube_namespace:datadog-agent-helm",
			"pod_name:datadog-agent-linux-hn9f2",
			"kube_container_name:security-agent",
		},
		"container_id://agent-04": {
			"kube_namespace:datadog-agent-helm",
			"pod_name:datadog-agent-linux-hn9f2",
			"kube_container_name:non-existing-in-summary",
		},
		"container_id://80bd9ebe296615341c68d571e843d800fb4a75bef696d858065572ab4e49920b": {
			"kube_namespace:default",
			"pod_name:long-running-init",
			"kube_container_name:init",
		},
		"container_id://not-yet-running-so-empty": {
			"kube_namespace:default",
			"pod_name:long-running-init",
			"kube_container_name:main-app",
		},
	}
)

func TestProvider_Provide(t *testing.T) {
	useStatsSummaryAsSource := true

	type metrics struct {
		name  string
		value float64
		tags  []string
	}

	type response struct {
		code int
		err  error
	}
	type want struct {
		gaugeMetrics []metrics
		rateMetrics  []metrics
		err          error
	}
	tests := []struct {
		name     string
		response response
		want     want
	}{
		{
			name: "endpoint 404 error",
			response: response{
				code: 404,
				err:  errors.New("Unable to fetch stats summary from Kubelet, rc: 404"),
			},
			want: want{
				gaugeMetrics: nil,
				rateMetrics:  nil,
				err:          errors.New("Unable to fetch stats summary from Kubelet, rc: 404"),
			},
		},
		{
			name: "endpoint no error",
			response: response{
				code: 200,
				err:  nil,
			},
			want: want{
				gaugeMetrics: []metrics{
					{
						name:  common.KubeletMetricsPrefix + "node.filesystem.usage",
						value: 14517391360,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "node.filesystem.usage_pct",
						value: 0.13977132820196123,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "node.image.filesystem.usage",
						value: 10980311040,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "node.image.filesystem.usage_pct",
						value: 0.10571683438666064,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "kubelet.memory.rss",
						value: 77819904,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "kubelet.memory.usage",
						value: 147464192,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "kubelet.cpu.usage",
						value: 42549809,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "runtime.memory.rss",
						value: 219836416,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "runtime.memory.usage",
						value: 1521729536,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "runtime.cpu.usage",
						value: 58035303,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "ephemeral_storage.usage",
						value: 17596416,
						tags:  []string{"instance_tag:something", "app:datadog-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage",
						value: 94208,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage_pct",
						value: 9.070208938178244e-07,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage",
						value: 102400,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:security-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage_pct",
						value: 9.858922758889397e-07,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:security-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage",
						value: 69632,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:process-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage_pct",
						value: 6.704067476044789e-07,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:process-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "ephemeral_storage.usage",
						value: 32768,
						tags:  []string{"instance_tag:something", "app:pending-pod"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage",
						value: 28672,
						tags: []string{"instance_tag:something", "kube_namespace:default",
							"pod_name:long-running-init", "kube_container_name:init"},
					},
					{
						name:  common.KubeletMetricsPrefix + "filesystem.usage_pct",
						value: 1.4589260524364285e-07,
						tags: []string{"instance_tag:something", "kube_namespace:default",
							"pod_name:long-running-init", "kube_container_name:init"},
					},
				},
				rateMetrics: []metrics{
					{
						name:  common.KubeletMetricsPrefix + "network.tx_bytes",
						value: 958986495,
						tags:  []string{"instance_tag:something", "app:datadog-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "network.rx_bytes",
						value: 870160903,
						tags:  []string{"instance_tag:something", "app:datadog-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "cpu.usage.total",
						value: 45241120000,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.working_set",
						value: 80461824,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.usage",
						value: 85618688,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "cpu.usage.total",
						value: 6361765000,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:security-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.working_set",
						value: 52338688,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:security-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.usage",
						value: 52842496,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:security-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "cpu.usage.total",
						value: 6903135000,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:process-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.working_set",
						value: 68685824,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:process-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.usage",
						value: 69447680,
						tags: []string{"instance_tag:something", "kube_namespace:datadog-agent-helm",
							"pod_name:datadog-agent-linux-hn9f2", "kube_container_name:process-agent"},
					},
					{
						name:  common.KubeletMetricsPrefix + "network.tx_bytes",
						value: 1146,
						tags:  []string{"instance_tag:something", "app:pending-pod"},
					},
					{
						name:  common.KubeletMetricsPrefix + "network.rx_bytes",
						value: 0,
						tags:  []string{"instance_tag:something", "app:pending-pod"},
					},
					{
						name:  common.KubeletMetricsPrefix + "cpu.usage.total",
						value: 9927000,
						tags: []string{"instance_tag:something", "kube_namespace:default",
							"pod_name:long-running-init", "kube_container_name:init"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.working_set",
						value: 229376,
						tags: []string{"instance_tag:something", "kube_namespace:default",
							"pod_name:long-running-init", "kube_container_name:init"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.usage",
						value: 229376,
						tags: []string{"instance_tag:something", "kube_namespace:default",
							"pod_name:long-running-init", "kube_container_name:init"},
					},
				},
				err: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
			mockSender.SetupAcceptAll()

			fakeTagger := taggerimpl.SetupFakeTagger(t)

			for entity, tags := range entityTags {
				prefix, id, _ := taggercommon.ExtractPrefixAndID(entity)
				entityID := taggertypes.NewEntityID(prefix, id)
				fakeTagger.SetTags(entityID, "foo", tags, nil, nil, nil)
			}
			store := creatFakeStore(t)
			kubeletMock := mock.NewKubeletMock()
			setFakeStatsSummary(t, kubeletMock, tt.response.code, tt.response.err)

			config := &common.KubeletConfig{
				OpenmetricsInstance: types.OpenmetricsInstance{
					Tags: []string{"instance_tag:something"},
				},
				EnabledRates: []string{ //default
					"diskio.io_service_bytes.stats.total",
					"network[\\.].._bytes",
					"cpu[\\.].*[\\.]total",
					"+cpu[\\.].*[\\.]total", //invalid regexp, should be skipped in NewProvider.
				},
				UseStatsSummaryAsSource: &useStatsSummaryAsSource,
			}

			p := NewProvider(
				&containers.Filter{
					Enabled: true,
				},
				config,
				store,
				fakeTagger,
			)
			assert.NoError(t, err)

			err = p.Provide(kubeletMock, mockSender)
			if !reflect.DeepEqual(err, tt.want.err) {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.want.gaugeMetrics))
			mockSender.AssertNumberOfCalls(t, "Rate", len(tt.want.rateMetrics))
			for _, metric := range tt.want.gaugeMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}
			for _, metric := range tt.want.rateMetrics {
				mockSender.AssertMetric(t, "Rate", metric.name, metric.value, "", metric.tags)
			}
		})
	}
}

func creatFakeStore(t *testing.T) workloadmetamock.Mock {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		configcomp.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	podEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "foobar",
	}
	pod := &workloadmeta.KubernetesPod{
		EntityID: podEntityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "datadog-agent-1234",
			Namespace: "default",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "agent-01",
				Name: "agent",
			},
			{
				ID:   "agent-02",
				Name: "process-agent",
			},
			{
				ID:   "agent-03",
				Name: "security-agent",
			},
			{
				ID:   "agent-04",
				Name: "non-existing-in-summary",
			},
		},
		Ready:         true,
		Phase:         "Running",
		IP:            "127.0.0.1",
		PriorityClass: "some_priority",
		QOSClass:      "guaranteed",
	}
	store.Set(pod)

	pendingPodEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "d2dfd16e-e829-4f66-91d5-f9233ca7332b",
	}
	pendingPod := &workloadmeta.KubernetesPod{
		EntityID: pendingPodEntityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "long-running-init",
			Namespace: "default",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "80bd9ebe296615341c68d571e843d800fb4a75bef696d858065572ab4e49920b",
				Name: "init",
			},
			{
				ID:   "not-yet-running-so-empty",
				Name: "main-app",
			},
		},
		Ready:         false,
		Phase:         "Pending",
		IP:            "10.244.0.122",
		PriorityClass: "some_priority",
		QOSClass:      "BestEffort",
	}
	store.Set(pendingPod)
	return store
}

func setFakeStatsSummary(t *testing.T, kubeletMock *mock.KubeletMock, rc int, err error) {
	filePath := "../../testdata/summary.json"
	content, fileErr := os.ReadFile(filePath)
	if fileErr != nil {
		t.Errorf("unable to read test file at: %s, err: %v", filePath, fileErr)
	}
	kubeletMock.MockReplies["/stats/summary"] = &mock.HTTPReplyMock{
		Data:         content,
		ResponseCode: rc,
		Error:        err,
	}
}
