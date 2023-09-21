// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package summary

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

var (
	//entity id: [fake tags]
	entityTags = map[string][]string{
		//pod name in summary: datadog-agent-linux-hn9f2
		"kubernetes_pod_uid://foobar": {
			"app:datadog-agent",
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

			fakeTagger := local.NewFakeTagger()
			for entity, tags := range entityTags {
				fakeTagger.SetTags(entity, "foo", tags, nil, nil, nil)
			}
			tagger.SetDefaultTagger(fakeTagger)
			store := creatFakeStore()
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

func creatFakeStore() *workloadmetatesting.Store {
	store := workloadmetatesting.NewStore()
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
