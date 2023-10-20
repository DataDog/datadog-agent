// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	tmock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

var (
	commonTags = map[string][]string{
		"kubernetes_pod_uid://c2319815-10d0-11e8-bd5a-42010af00137": {"pod_name:datadog-agent-jbm2k"},
		"kubernetes_pod_uid://2edfd4d9-10ce-11e8-bd5a-42010af00137": {"pod_name:fluentd-gcp-v2.0.10-9q9t4"},
		"kubernetes_pod_uid://2fdfd4d9-10ce-11e8-bd5a-42010af00137": {"pod_name:fluentd-gcp-v2.0.10-p13r3"},
		"container_id://5741ed2471c0e458b6b95db40ba05d1a5ee168256638a0264f08703e48d76561": {
			"kube_container_name:fluentd-gcp",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://580cb469826a10317fd63cc780441920f49913ae63918d4c7b19a72347645b05": {
			"kube_container_name:prometheus-to-sd-exporter",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://6941ed2471c0e458b6b95db40ba05d1a5ee168256638a0264f08703e48d76561": {
			"kube_container_name:fluentd-gcp",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://690cb469826a10317fd63cc780441920f49913ae63918d4c7b19a72347645b05": {
			"kube_container_name:prometheus-to-sd-exporter",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://5f93d91c7aee0230f77fbe9ec642dd60958f5098e76de270a933285c24dfdc6f": {
			"pod_name:demo-app-success-c485bc67b-klj45",
		},
		"kubernetes_pod_uid://d2e71e36-10d0-11e8-bd5a-42010af00137": {"pod_name:dd-agent-q6hpw"},
		"kubernetes_pod_uid://260c2b1d43b094af6d6b4ccba082c2db": {
			"pod_name:kube-proxy-gke-haissam-default-pool-be5066f1-wnvn",
		},
		"kubernetes_pod_uid://24d6daa3-10d8-11e8-bd5a-42010af00137":                       {"pod_name:demo-app-success-c485bc67b-klj45"},
		"container_id://f69aa93ce78ee11e78e7c75dc71f535567961740a308422dafebdb4030b04903": {"pod_name:pi-kff76"},
		"kubernetes_pod_uid://12ceeaa9-33ca-11e6-ac8f-42010af00003":                       {"pod_name:dd-agent-ntepl"},
		"container_id://32fc50ecfe24df055f6d56037acb966337eef7282ad5c203a1be58f2dd2fe743": {"pod_name:dd-agent-ntepl"},
		"container_id://a335589109ce5506aa69ba7481fc3e6c943abd23c5277016c92dac15d0f40479": {
			"kube_container_name:datadog-agent",
		},
		"container_id://326b384481ca95204018e3e837c61e522b64a3b86c3804142a22b2d1db9dbd7b": {
			"kube_container_name:datadog-agent",
		},
		"container_id://6d8c6a05731b52195998c438fdca271b967b171f6c894f11ba59aa2f4deff10c": {"pod_name:cassandra-0"},
		"kubernetes_pod_uid://639980e5-2e6c-11ea-8bb1-42010a800074": {
			"kube_namespace:default",
			"kube_service:nginx",
			"kube_stateful_set:web",
			"namespace:default",
			"persistentvolumeclaim:www-web-2",
			"pod_phase:running",
		},
		"kubernetes_pod_uid://639980e5-2e6c-11ea-8bb1-42010a800075": {
			"kube_namespace:default",
			"kube_service:nginx",
			"kube_stateful_set:web",
			"namespace:default",
			"persistentvolumeclaim:www-web-2",
			"persistentvolumeclaim:www2-web-3",
			"pod_phase:running",
		},
	}

	instanceTags = []string{"instance_tag:something"}

	expectedMetricsPrometheus = []string{
		common.KubeletMetricsPrefix + "apiserver.certificate.expiration.count",
		common.KubeletMetricsPrefix + "apiserver.certificate.expiration.sum",
		common.KubeletMetricsPrefix + "rest.client.requests",
		common.KubeletMetricsPrefix + "rest.client.latency.count",
		common.KubeletMetricsPrefix + "rest.client.latency.sum",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations",
		common.KubeletMetricsPrefix + "kubelet.runtime.errors",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations.duration.sum",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations.duration.count",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations.duration.quantile",
		common.KubeletMetricsPrefix + "kubelet.docker.operations",
		common.KubeletMetricsPrefix + "kubelet.docker.errors",
		common.KubeletMetricsPrefix + "kubelet.docker.operations.duration.sum",
		common.KubeletMetricsPrefix + "kubelet.docker.operations.duration.count",
		common.KubeletMetricsPrefix + "kubelet.docker.operations.duration.quantile",
		common.KubeletMetricsPrefix + "kubelet.network_plugin.latency.sum",
		common.KubeletMetricsPrefix + "kubelet.network_plugin.latency.count",
		common.KubeletMetricsPrefix + "kubelet.volume.stats.available_bytes",
		common.KubeletMetricsPrefix + "kubelet.volume.stats.capacity_bytes",
		common.KubeletMetricsPrefix + "kubelet.volume.stats.used_bytes",
		common.KubeletMetricsPrefix + "kubelet.volume.stats.inodes",
		common.KubeletMetricsPrefix + "kubelet.volume.stats.inodes_free",
		common.KubeletMetricsPrefix + "kubelet.volume.stats.inodes_used",
		common.KubeletMetricsPrefix + "kubelet.evictions",
		common.KubeletMetricsPrefix + "kubelet.pod.start.duration.sum",
		common.KubeletMetricsPrefix + "kubelet.pod.start.duration.count",
		common.KubeletMetricsPrefix + "kubelet.pod.worker.start.duration.sum",
		common.KubeletMetricsPrefix + "kubelet.pod.worker.start.duration.count",
		common.KubeletMetricsPrefix + "go_threads",
		common.KubeletMetricsPrefix + "go_goroutines",
	}

	expectedMetricsPrometheus114 = append(expectedMetricsPrometheus,
		common.KubeletMetricsPrefix+"kubelet.container.log_filesystem.used_bytes",
		common.KubeletMetricsPrefix+"kubelet.pod.worker.duration.sum",
		common.KubeletMetricsPrefix+"kubelet.pod.worker.duration.count",
		common.KubeletMetricsPrefix+"kubelet.pleg.relist_duration.count",
		common.KubeletMetricsPrefix+"kubelet.pleg.relist_duration.sum",
		common.KubeletMetricsPrefix+"kubelet.pleg.relist_interval.count",
		common.KubeletMetricsPrefix+"kubelet.pleg.relist_interval.sum",
	)

	expectedMetricsPrometheusPre114 = append(expectedMetricsPrometheus,
		common.KubeletMetricsPrefix+"kubelet.network_plugin.latency.quantile",
		common.KubeletMetricsPrefix+"kubelet.pod.start.duration.quantile",
		common.KubeletMetricsPrefix+"kubelet.pod.worker.start.duration.quantile",
	)

	expectedMetricsPrometheus121 = []string{
		common.KubeletMetricsPrefix + "apiserver.certificate.expiration.count",
		common.KubeletMetricsPrefix + "apiserver.certificate.expiration.sum",
		common.KubeletMetricsPrefix + "go_goroutines",
		common.KubeletMetricsPrefix + "go_threads",
		common.KubeletMetricsPrefix + "kubelet.container.log_filesystem.used_bytes",
		common.KubeletMetricsPrefix + "kubelet.network_plugin.latency.count",
		common.KubeletMetricsPrefix + "kubelet.network_plugin.latency.sum",
		common.KubeletMetricsPrefix + "kubelet.pleg.discard_events",
		common.KubeletMetricsPrefix + "kubelet.pleg.last_seen",
		common.KubeletMetricsPrefix + "kubelet.pleg.relist_duration.count",
		common.KubeletMetricsPrefix + "kubelet.pleg.relist_duration.sum",
		common.KubeletMetricsPrefix + "kubelet.pleg.relist_interval.count",
		common.KubeletMetricsPrefix + "kubelet.pleg.relist_interval.sum",
		common.KubeletMetricsPrefix + "kubelet.pod.start.duration.count",
		common.KubeletMetricsPrefix + "kubelet.pod.start.duration.sum",
		common.KubeletMetricsPrefix + "kubelet.pod.worker.start.duration.count",
		common.KubeletMetricsPrefix + "kubelet.pod.worker.start.duration.sum",
		common.KubeletMetricsPrefix + "kubelet.runtime.errors",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations.duration.count",
		common.KubeletMetricsPrefix + "kubelet.runtime.operations.duration.sum",
		common.KubeletMetricsPrefix + "rest.client.latency.count",
		common.KubeletMetricsPrefix + "rest.client.latency.sum",
		common.KubeletMetricsPrefix + "rest.client.requests",
		common.KubeletMetricsPrefix + "kubelet.evictions",
	}
)

type endpointResponse struct {
	filename string
	code     int
	err      error
}

type ProviderTestSuite struct {
	suite.Suite
	provider   *Provider
	mockSender *mocksender.MockSender
	store      *workloadmetatesting.Store
}

func (suite *ProviderTestSuite) SetupTest() {
	var err error

	mockSender := mocksender.NewMockSender(checkid.ID(suite.T().Name()))
	mockSender.SetupAcceptAll()
	suite.mockSender = mockSender

	fakeTagger := local.NewFakeTagger()
	for entity, tags := range commonTags {
		fakeTagger.SetTags(entity, "foo", tags, nil, nil, nil)
	}
	tagger.SetDefaultTagger(fakeTagger)

	podUtils := common.NewPodUtils()

	podsFile := "../../testdata/pods.json"
	store, err := storePopulatedFromFile(podsFile, podUtils)
	if err != nil {
		suite.T().Errorf("unable to populate store from file at: %s, err: %v", podsFile, err)
	}
	suite.store = store

	sendBuckets := true
	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags:                 instanceTags,
			SendHistogramBuckets: &sendBuckets,
			Namespace:            common.KubeletMetricsPrefix,
		},
	}

	p, err := NewProvider(
		&containers.Filter{
			Enabled:         true,
			NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
		},
		config,
		store,
		podUtils,
	)
	assert.NoError(suite.T(), err)
	suite.provider = p
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}

func (suite *ProviderTestSuite) TestExpectedMetricsShowUp() {
	type want struct {
		metrics []string
		err     error
	}
	tests := []struct {
		name     string
		response endpointResponse
		want     want
	}{
		{
			name: "pre 1.14 metrics all show up",
			response: endpointResponse{
				filename: "../../testdata/kubelet_metrics.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetricsPrometheusPre114,
			},
		},
		{
			name: "1.14 metrics all show up",
			response: endpointResponse{
				filename: "../../testdata/kubelet_metrics_1_14.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetricsPrometheus114,
			},
		},
		{
			name: "1.21 metrics all show up",
			response: endpointResponse{
				filename: "../../testdata/kubelet_metrics_1_21.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetricsPrometheus121,
			},
		},
	}
	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			suite.SetupTest()
			kubeletMock, err := createKubeletMock(tt.response)
			if err != nil {
				suite.T().Fatalf("error created kubelet mock: %v", err)
			}

			err = suite.provider.Provide(kubeletMock, suite.mockSender)
			if !reflect.DeepEqual(err, tt.want.err) {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}

			suite.assertMetricCallsMatch(t, tt.want.metrics)
		})
	}
}

func (suite *ProviderTestSuite) TestPodTagsOnPVCMetrics() {
	response := endpointResponse{
		filename: "../../testdata/kubelet_metrics.txt",
		code:     200,
		err:      nil,
	}

	kubeletMock, err := createKubeletMock(response)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// pvc tags show up
	podWithPVCTags := append(instanceTags, "persistentvolumeclaim:www-web-2", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithPVCTags)

	// ephemeral volume tags show up
	podWithEphemeralTags := append(instanceTags, "persistentvolumeclaim:web-2-ephemeralvolume", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithEphemeralTags)
}

func (suite *ProviderTestSuite) TestPVCMetricsExcludedByNamespace() {
	response := endpointResponse{
		filename: "../../testdata/kubelet_metrics.txt",
		code:     200,
		err:      nil,
	}

	kubeletMock, err := createKubeletMock(response)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	suite.provider.filter = &containers.Filter{
		NamespaceExcludeList: []*regexp.Regexp{regexp.MustCompile("default")},
		Enabled:              true,
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// pvc tags show up
	podWithPVCTags := append(instanceTags, "persistentvolumeclaim:www-web-2", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithPVCTags)

	// ephemeral volume tags show up
	podWithEphemeralTags := append(instanceTags, "persistentvolumeclaim:web-2-ephemeralvolume", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithEphemeralTags)
}

func (suite *ProviderTestSuite) TestSendAlwaysCounter() {
	response := endpointResponse{
		filename: "../../testdata/kubelet_metrics_1_21.txt",
		code:     200,
		err:      nil,
	}

	kubeletMock, err := createKubeletMock(response)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// expected counters show up
	suite.mockSender.AssertMetric(suite.T(), "MonotonicCount", common.KubeletMetricsPrefix+"kubelet.evictions", 3, "", append(instanceTags, "eviction_signal:allocatableMemory.available"))
	suite.mockSender.AssertMetric(suite.T(), "MonotonicCount", common.KubeletMetricsPrefix+"kubelet.evictions", 3, "", append(instanceTags, "eviction_signal:memory.available"))
	suite.mockSender.AssertMetric(suite.T(), "MonotonicCount", common.KubeletMetricsPrefix+"kubelet.pleg.discard_events", 0, "", instanceTags)
}

func (suite *ProviderTestSuite) TestKubeletContainerLogFilesystemUsedBytes() {
	// Get around floating point conversion issues during AssertCalled
	expected, _ := strconv.ParseFloat("24576", 64)

	response := endpointResponse{
		filename: "../../testdata/kubelet_metrics_1_21.txt",
		code:     200,
		err:      nil,
	}

	kubeletMock, err := createKubeletMock(response)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// container id has tags, so container tags show up
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.container.log_filesystem.used_bytes", 5242822656, "", append(instanceTags, "kube_container_name:datadog-agent"))
	// container id not found in tagger, so no container tags show up
	suite.mockSender.AssertCalled(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.container.log_filesystem.used_bytes", expected, "", instanceTags)
}

func (suite *ProviderTestSuite) TestRestClientLatency() {
	response := endpointResponse{
		filename: "../../testdata/kubelet_metrics_1_21.txt",
		code:     200,
		err:      nil,
	}

	kubeletMock, err := createKubeletMock(response)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// url is parsed
	// note: there are so many metric points generated for this metric based on the input data, we are just going to focus on one
	expectedTags := append(instanceTags, "url:/api/v1/namespaces/{namespace}/configmaps", "verb:GET")
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.001"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.002"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.004"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.008"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.016"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.032"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.064"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.128"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.256"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.count", append(expectedTags, "upper_bound:0.512"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"rest.client.latency.sum", expectedTags)
}

func (suite *ProviderTestSuite) TestHistogramFromSecondsToMicroseconds() {
	response := endpointResponse{
		filename: "../../testdata/kubelet_metrics_1_21.txt",
		code:     200,
		err:      nil,
	}

	kubeletMock, err := createKubeletMock(response)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// upper_bound tag is transformed for buckets
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.network_plugin.latency.count", 14, "", append(instanceTags, "operation_type:get_pod_network_status", "upper_bound:5000.000000"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.start.duration.count", 30, "", append(instanceTags, "upper_bound:5000.000000"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.worker.start.duration.count", 30, "", append(instanceTags, "upper_bound:5000.000000"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.runtime.operations.duration.count", 177, "", append(instanceTags, "operation_type:container_status", "upper_bound:5000.000000"))

	// value is transformed for sum
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.network_plugin.latency.sum", 1.1268392169999992e+06, "", append(instanceTags, "operation_type:get_pod_network_status"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.start.duration.sum", 202368874.00600008, "", instanceTags)
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.worker.start.duration.sum", 26680.296, "", instanceTags)
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.runtime.operations.duration.sum", 1204396.2709999991, "", append(instanceTags, "operation_type:container_status"))
}

// assertMetricCallsMatch is a helper function which allows us to assert that, for a given test and a given set of expected
// metrics, ONLY the expected metrics have been called, and ALL the expected metrics have been called.
func (suite *ProviderTestSuite) assertMetricCallsMatch(t *testing.T, expectedMetrics []string) {
	// note: this is awful and ugly, but it works for now
	var matchedAsserts []tmock.Call
	// Make sure that every metric in the expectedMetrics slice has been called
	for _, expectedMetric := range expectedMetrics {
		matches := 0
		for _, call := range suite.mockSender.Calls {
			expected := tmock.Arguments{expectedMetric, tmock.AnythingOfType("float64"), "", mocksender.MatchTagsContains(instanceTags)}
			if _, diffs := expected.Diff(call.Arguments); diffs == 0 {
				matches++
				matchedAsserts = append(matchedAsserts, call)
			}
		}
		if matches == 0 {
			t.Errorf("expected metric %s to be called, but it was not", expectedMetric)
		}
	}

	// find out output any actual calls which exist which were not in the expected list
	if len(matchedAsserts) != len(suite.mockSender.Calls) {
		var calledWithArgs []string
		for _, call := range suite.mockSender.Calls {
			wasMatched := false
			for _, matched := range matchedAsserts {
				if call.Method == matched.Method {
					if _, diffs := matched.Arguments.Diff(call.Arguments); diffs == 0 {
						wasMatched = true
						break
					}
				}
			}
			if !wasMatched {
				calledWithArgs = append(calledWithArgs, fmt.Sprintf("%v", call.Arguments))
			}
		}
		t.Errorf("expected %v metrics to be matched, but %v were", len(suite.mockSender.Calls), len(matchedAsserts))
		t.Errorf("missing assertions for calls:\n        %v", strings.Join(calledWithArgs, "\n"))
	}
}

func createKubeletMock(response endpointResponse) (*mock.KubeletMock, error) {
	var err error

	kubeletMock := mock.NewKubeletMock()
	var content []byte
	if response.filename != "" {
		content, err = os.ReadFile(response.filename)
		if err != nil {
			return nil, fmt.Errorf(fmt.Sprintf("unable to read test file at: %s, err: %v", response.filename, err))
		}
	}
	kubeletMock.MockReplies["/metrics"] = &mock.HTTPReplyMock{
		Data:         content,
		ResponseCode: response.code,
		Error:        response.err,
	}
	return kubeletMock, nil
}

func storePopulatedFromFile(filename string, podUtils *common.PodUtils) (*workloadmetatesting.Store, error) {
	store := workloadmetatesting.NewStore()

	if filename == "" {
		return store, nil
	}

	podList, err := os.ReadFile(filename)
	if err != nil {
		return store, fmt.Errorf(fmt.Sprintf("unable to load pod list, err: %v", err))
	}
	var pods *kubelet.PodList
	err = json.Unmarshal(podList, &pods)
	if err != nil {
		return store, fmt.Errorf(fmt.Sprintf("unable to load pod list, err: %v", err))
	}

	for _, pod := range pods.Items {
		podContainers := make([]workloadmeta.OrchestratorContainer, 0, len(pod.Status.Containers))

		for _, container := range pod.Status.Containers {
			if container.ID == "" {
				// A container without an ID has not been created by
				// the runtime yet, so we ignore them until it's
				// detected again.
				continue
			}

			image, err := workloadmeta.NewContainerImage(container.ImageID, container.Image)
			if err != nil {
				if err == containers.ErrImageIsSha256 {
					// try the resolved image ID if the image name in the container
					// status is a SHA256. this seems to happen sometimes when
					// pinning the image to a SHA256
					image, _ = workloadmeta.NewContainerImage(container.ImageID, container.ImageID)
				}
			}

			_, containerID := containers.SplitEntityName(container.ID)
			podContainer := workloadmeta.OrchestratorContainer{
				ID:   containerID,
				Name: container.Name,
			}
			podContainer.Image, _ = workloadmeta.NewContainerImage(container.ImageID, container.Image)

			podContainer.Image.ID = container.ImageID

			podContainers = append(podContainers, podContainer)
			store.Set(&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: container.Name,
					Labels: map[string]string{
						kubernetes.CriContainerNamespaceLabel: pod.Metadata.Namespace,
					},
				},
				Image: image,
			})
		}

		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   pod.Metadata.UID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        pod.Metadata.Name,
				Namespace:   pod.Metadata.Namespace,
				Annotations: pod.Metadata.Annotations,
				Labels:      pod.Metadata.Labels,
			},
			Containers: podContainers,
		})
		podUtils.ComputePodTagsByPVC(pod)
	}
	return store, err
}
