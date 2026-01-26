// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package summary

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	kubeletmock "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
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
						name:  common.KubeletMetricsPrefix + "cpu.usage.total",
						value: 6361765000,
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

			fakeTagger := taggerfxmock.SetupFakeTagger(t)

			for entity, tags := range entityTags {
				prefix, id, _ := taggertypes.ExtractPrefixAndID(entity)
				entityID := taggertypes.NewEntityID(prefix, id)
				fakeTagger.SetTags(entityID, "foo", tags, nil, nil, nil)
			}
			store := creatFakeStore(t)
			kubeletMock := kubeletmock.NewKubeletMock()
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
			mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

			p := NewProvider(
				mockFilterStore,
				config,
				store,
				fakeTagger,
			)
			assert.NoError(t, err)

			err = p.Provide(kubeletMock, mockSender)
			if tt.want.err != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.want.err.Error(), err.Error())
			} else {
				assert.NoError(t, err)
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
		fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
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

func setFakeStatsSummary(t *testing.T, kubeletMock *kubeletmock.KubeletMock, rc int, err error) {
	filePath := "../../testdata/summary.json"
	content, fileErr := os.ReadFile(filePath)
	if fileErr != nil {
		t.Errorf("unable to read test file at: %s, err: %v", filePath, fileErr)
	}
	kubeletMock.MockReplies["/stats/summary"] = &kubeletmock.HTTPReplyMock{
		Data:         content,
		ResponseCode: rc,
		Error:        err,
	}
}

// FilteringTestSuite tests filtering functionality of the summary provider.
type FilteringTestSuite struct {
	suite.Suite
	store          workloadmeta.Component
	workloadFilter workloadfilter.Component
	provider       *Provider
	mockSender     *mocksender.MockSender
	mockKubelet    *kubeletmock.KubeletMock
	mockConfig     model.Config
}

func (suite *FilteringTestSuite) SetupTest() {
	store := fxutil.Test[workloadmetamock.Mock](suite.T(), fx.Options(
		fx.Provide(func() log.Component { return logmock.New(suite.T()) }),
		fx.Provide(func() configcomp.Component { return configcomp.NewMock(suite.T()) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	suite.store = store
	suite.mockSender = mocksender.NewMockSender(checkid.ID("test"))
	suite.mockSender.SetupAcceptAll()

	// Set up kubelet mock
	suite.mockKubelet = kubeletmock.NewKubeletMock()

	// Create mock config for filter configuration
	suite.mockConfig = configmock.New(suite.T())
	suite.workloadFilter = workloadfilterfxmock.SetupMockFilter(suite.T())
}

func (suite *FilteringTestSuite) createProviderWithFilters() {
	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())

	// Set up tags for the test data pods using actual UIDs from the test data
	podTags := map[string][]string{
		"regular-pod-uid-123":   {"kube_namespace:default", "pod_name:regular-pod"},
		"annotated-pod-uid-456": {"kube_namespace:kube-system", "pod_name:annotated-pod"},
		"pause-pod-uid-789":     {"kube_namespace:kube-system", "pod_name:pause-pod-test"},
		"filtered-pod-uid-101":  {"kube_namespace:production", "pod_name:filtered-container-pod"},
	}

	for podUID, tags := range podTags {
		podEntityID := taggertypes.NewEntityID(taggertypes.KubernetesPodUID, podUID)
		fakeTagger.SetTags(podEntityID, "orchestrator", tags, nil, nil, nil)
	}

	// Set up container tags using actual container IDs from the test data
	containerTags := map[string][]string{
		"regular-container-id-123": {"kube_namespace:default", "pod_name:regular-pod", "kube_container_name:app-container"},
		"system-container-id-456":  {"kube_namespace:kube-system", "pod_name:annotated-pod", "kube_container_name:system-container"},
		"pause-container-id-789":   {"kube_namespace:kube-system", "pod_name:pause-pod-test", "kube_container_name:paused-container", "image_name:kubernetes/pause:3.2"},
		"main-app-id-101":          {"kube_namespace:production", "pod_name:filtered-container-pod", "kube_container_name:main-app"},
		"istio-proxy-id-101":       {"kube_namespace:production", "pod_name:filtered-container-pod", "kube_container_name:istio-proxy", "image_name:istio/proxyv2:1.15.0"},
	}

	for containerID, tags := range containerTags {
		containerEntityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
		fakeTagger.SetTags(containerEntityID, "high", tags, nil, nil, nil)
	}

	suite.workloadFilter = workloadfilterfxmock.SetupMockFilter(suite.T())

	// Create provider with the current config
	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags:      []string{"instance_tag:something"},
			Namespace: common.KubeletMetricsPrefix,
		},
		UseStatsSummaryAsSource: &[]bool{true}[0],
	}
	suite.provider = NewProvider(suite.workloadFilter, config, suite.store, fakeTagger)
}

func (suite *FilteringTestSuite) setupFilteredTestData() {
	// Load test summary data
	summaryPath := "../../testdata/summary_filtered.json"
	summaryContent, err := os.ReadFile(summaryPath)
	suite.Require().NoError(err)

	suite.mockKubelet.MockReplies["/stats/summary"] = &kubeletmock.HTTPReplyMock{
		Data:         summaryContent,
		ResponseCode: 200,
		Error:        nil,
	}

	// Load pod test data
	podsPath := "../../testdata/pods_summary_filtered.json"
	podsContent, err := os.ReadFile(podsPath)
	suite.Require().NoError(err)

	var podData []struct {
		UID        string `json:"uid"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Phase      string `json:"phase"`
		Containers []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Image struct {
				Name string `json:"name"`
			} `json:"image"`
		} `json:"containers"`
		Annotations map[string]string `json:"annotations"`
		Labels      map[string]string `json:"labels"`
	}
	err = json.Unmarshal(podsContent, &podData)
	suite.Require().NoError(err)

	// Populate workloadmeta store
	mockStore := suite.store.(workloadmetamock.Mock)
	for _, pod := range podData {
		var containers []workloadmeta.OrchestratorContainer
		for _, container := range pod.Containers {
			containers = append(containers, workloadmeta.OrchestratorContainer{
				ID:   container.ID,
				Name: container.Name,
				Image: workloadmeta.ContainerImage{
					RawName: container.Image.Name,
				},
			})
		}

		wmPod := &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   pod.UID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        pod.Name,
				Namespace:   pod.Namespace,
				Annotations: pod.Annotations,
				Labels:      pod.Labels,
			},
			Containers: containers,
			Phase:      pod.Phase,
		}
		mockStore.Set(wmPod)
	}
}

func (suite *FilteringTestSuite) TestPodAnnotationFiltering() {
	suite.setupFilteredTestData()

	// No need to configure anything - ad.datadoghq.com/exclude annotation works automatically
	suite.createProviderWithFilters()

	// Provide metrics
	err := suite.provider.Provide(suite.mockKubelet, suite.mockSender)
	suite.Assert().NoError(err)

	// Verify that annotated-pod metrics are excluded (pods with ad.datadoghq.com/exclude: "true")
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral_storage.usage", []string{"pod_name:annotated-pod"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", []string{"pod_name:annotated-pod"})

	// Verify that regular pods still have metrics
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral_storage.usage", []string{"pod_name:regular-pod"})
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", []string{"pod_name:regular-pod"})

	// Reset for next test
	suite.mockSender.ResetCalls()
}

func (suite *FilteringTestSuite) TestContainerNameFiltering() {
	suite.setupFilteredTestData()

	// Configure workload filter to exclude istio-proxy containers using config
	suite.mockConfig.SetWithoutSource("container_exclude", "name:istio-proxy")
	suite.createProviderWithFilters()

	// Provide metrics
	err := suite.provider.Provide(suite.mockKubelet, suite.mockSender)
	suite.Assert().NoError(err)

	// Verify that istio-proxy container metrics are excluded
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:istio-proxy"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"cpu.usage.total", []string{"kube_container_name:istio-proxy"})

	// Verify that main-app container still has metrics
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:main-app"})
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"cpu.usage.total", []string{"kube_container_name:main-app"})

	// Reset for next test
	suite.mockSender.ResetCalls()
}

func (suite *FilteringTestSuite) TestNamespaceFiltering() {
	suite.setupFilteredTestData()

	// Configure workload filter to exclude kube-system namespace pods using config
	suite.mockConfig.SetWithoutSource("container_exclude", "kube_namespace:kube-system")
	suite.createProviderWithFilters()

	// Provide metrics
	err := suite.provider.Provide(suite.mockKubelet, suite.mockSender)
	suite.Assert().NoError(err)

	// Verify that kube-system pods are excluded
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral_storage.usage", []string{"kube_namespace:kube-system"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", []string{"kube_namespace:kube-system"})

	// Verify that other namespaces still have metrics
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral_storage.usage", []string{"kube_namespace:default"})
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral_storage.usage", []string{"kube_namespace:production"})

	// Reset for next test
	suite.mockSender.ResetCalls()
}

func (suite *FilteringTestSuite) TestPauseContainerImageFiltering() {
	suite.setupFilteredTestData()

	// Configure workload filter to exclude pause containers using exact image name
	suite.createProviderWithFilters()

	// Provide metrics
	err := suite.provider.Provide(suite.mockKubelet, suite.mockSender)
	suite.Assert().NoError(err)

	// Verify that pause container metrics are excluded
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:paused-container"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.usage", []string{"kube_container_name:paused-container"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"filesystem.usage", []string{"kube_container_name:paused-container"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"cpu.usage.total", []string{"kube_container_name:paused-container"})

	// Verify that non-pause containers still have metrics
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:app-container"})
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:main-app"})

	// Reset for next test
	suite.mockSender.ResetCalls()
}

func (suite *FilteringTestSuite) TestSpecificImageNameFiltering() {
	suite.setupFilteredTestData()

	// Configure workload filter to exclude containers with specific image name
	suite.mockConfig.SetWithoutSource("container_exclude", "image:istio/proxy")
	suite.createProviderWithFilters()

	// Provide metrics
	err := suite.provider.Provide(suite.mockKubelet, suite.mockSender)
	suite.Assert().NoError(err)

	// Verify that istio-proxy container metrics are excluded
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:istio-proxy"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.usage", []string{"kube_container_name:istio-proxy"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"filesystem.usage", []string{"kube_container_name:istio-proxy"})
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"cpu.usage.total", []string{"kube_container_name:istio-proxy"})

	// Verify that other containers still have metrics
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:main-app"})
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.working_set", []string{"kube_container_name:app-container"})

	// Reset for next test
	suite.mockSender.ResetCalls()
}

func TestSummaryFilteringSuite(t *testing.T) {
	suite.Run(t, new(FilteringTestSuite))
}

// TestStaticPodUIDMismatchFallback tests that static pods are correctly handled
// when kubelet_use_api_server is enabled. In this case, workloadmeta stores pods
// with canonical UUIDs (from API server) but /stats/summary returns mirror hash UIDs
// (from kubelet). The provider should fall back to name/namespace lookup.
func TestStaticPodUIDMismatchFallback(t *testing.T) {
	// Canonical UUID (from API server, stored in workloadmeta)
	canonicalUID := "85a6cc02-4460-4f8a-b5f0-123456789abc"

	// Setup mock config with kubelet_use_api_server=true
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("kubelet_use_api_server", true)

	// Setup tagger with canonical UUID
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	entityID := taggertypes.NewEntityID(taggertypes.KubernetesPodUID, canonicalUID)
	fakeTagger.SetTags(entityID, "foo", []string{"app:kube-apiserver", "kube_namespace:kube-system"}, nil, nil, nil)

	// Setup workloadmeta store with pod using canonical UUID
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	staticPod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   canonicalUID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "kube-apiserver-test-node",
			Namespace: "kube-system",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "apiserver-container-01",
				Name: "kube-apiserver",
			},
		},
		Phase: "Running",
	}
	store.Set(staticPod)

	// Setup kubelet mock with summary containing mirror hash UID (from testdata file)
	kubeletMock := kubeletmock.NewKubeletMock()
	filePath := "../../testdata/summary_static_pod.json"
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("unable to read test file at: %s, err: %v", filePath, err)
	}
	kubeletMock.MockReplies["/stats/summary"] = &kubeletmock.HTTPReplyMock{
		Data:         content,
		ResponseCode: 200,
		Error:        nil,
	}

	// Setup sender
	mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
	mockSender.SetupAcceptAll()

	// Create provider
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)
	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags: []string{"instance_tag:test"},
		},
	}
	provider := NewProvider(mockFilterStore, config, store, fakeTagger)

	// Run provider
	err = provider.Provide(kubeletMock, mockSender)
	assert.NoError(t, err)

	// Verify metrics were collected for the static pod (fallback worked)
	// The tags should come from tagger using canonical UUID
	mockSender.AssertMetricTaggedWith(t, "Gauge",
		common.KubeletMetricsPrefix+"ephemeral_storage.usage",
		[]string{"app:kube-apiserver"})
}
