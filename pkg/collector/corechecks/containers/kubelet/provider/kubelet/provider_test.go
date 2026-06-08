// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	commontesting "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common/testing"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	endpoint = "/metrics"
)

var (
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
		common.KubeletMetricsPrefix + "kubelet.cpu_manager.pinning_errors_total",
		common.KubeletMetricsPrefix + "kubelet.cpu_manager.pinning_requests_total",
	}
)

type ProviderTestSuite struct {
	suite.Suite
	provider   *Provider
	mockSender *mocksender.MockSender
	store      workloadmeta.Component
	tagger     tagger.Component
}

func (suite *ProviderTestSuite) SetupTest() {
	var err error

	store := fxutil.Test[workloadmetamock.Mock](suite.T(), fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	mockSender := mocksender.NewMockSender(checkid.ID(suite.T().Name()))
	mockSender.SetupAcceptAll()
	suite.mockSender = mockSender

	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())

	for entity, tags := range commontesting.CommonTags {
		prefix, id, _ := taggertypes.ExtractPrefixAndID(entity)
		entityID := taggertypes.NewEntityID(prefix, id)
		fakeTagger.SetTags(entityID, "foo", tags, nil, nil, nil)
	}
	suite.tagger = fakeTagger

	podUtils := common.NewPodUtils(fakeTagger)

	podsFile := "../../testdata/pods.json"
	err = commontesting.StorePopulatedFromFile(store, podsFile, podUtils)
	if err != nil {
		suite.T().Errorf("unable to populate store from file at: %s, err: %v", podsFile, err)
	}
	suite.store = store

	sendBuckets := true
	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags:                 commontesting.InstanceTags,
			SendHistogramBuckets: &sendBuckets,
			Namespace:            common.KubeletMetricsPrefix,
		},
	}

	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("dd_container_exclude", "name:agent-excluded")
	mockConfig.SetInTest("dd_container_exclude_metrics", "image:^hkaj/demo-app$")
	mockConfig.SetInTest("dd_container_exclude_logs", "name:*") // should not affect metrics
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(suite.T())

	p, err := NewProvider(
		mockFilterStore,
		config,
		store,
		podUtils,
		fakeTagger,
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
		response commontesting.EndpointResponse
		want     want
	}{
		{
			name: "pre 1.14 metrics all show up",
			response: commontesting.NewEndpointResponse(
				"../../testdata/kubelet_metrics.txt", 200, nil),
			want: want{
				metrics: expectedMetricsPrometheusPre114,
			},
		},
		{
			name: "1.14 metrics all show up",
			response: commontesting.NewEndpointResponse(
				"../../testdata/kubelet_metrics_1_14.txt", 200, nil),
			want: want{
				metrics: expectedMetricsPrometheus114,
			},
		},
		{
			name: "1.21 metrics all show up",
			response: commontesting.NewEndpointResponse(
				"../../testdata/kubelet_metrics_1_21.txt", 200, nil),
			want: want{
				metrics: expectedMetricsPrometheus121,
			},
		},
	}
	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			suite.SetupTest()
			kubeletMock, err := commontesting.CreateKubeletMock(tt.response, endpoint)
			if err != nil {
				suite.T().Fatalf("error created kubelet mock: %v", err)
			}

			err = suite.provider.Provide(kubeletMock, suite.mockSender)
			if err != tt.want.err {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}

			commontesting.AssertMetricCallsMatch(t, tt.want.metrics, suite.mockSender)
		})
	}
}

func (suite *ProviderTestSuite) TestPodTagsOnPVCMetrics() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// pvc tags show up
	podWithPVCTags := append(commontesting.InstanceTags, "persistentvolumeclaim:www-web-2", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithPVCTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithPVCTags)

	// ephemeral volume tags show up
	podWithEphemeralTags := append(commontesting.InstanceTags, "persistentvolumeclaim:web-2-ephemeralvolume", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithEphemeralTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithEphemeralTags)
}

func (suite *ProviderTestSuite) TestPVCMetricsExcludedByNamespace() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("container_exclude", "kube_namespace:default")
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(suite.T())
	suite.provider.containerFilter = mockFilterStore.GetContainerSharedMetricFilters()
	suite.provider.podFilter = mockFilterStore.GetPodSharedMetricFilters()

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// namespace not filtered still shows up
	podWithPVCNotFilteredTags := append(commontesting.InstanceTags, "persistentvolumeclaim:ddagent-pvc-ddagent-test-2", "namespace:unit-test")

	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithPVCNotFilteredTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithPVCNotFilteredTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithPVCNotFilteredTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithPVCNotFilteredTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithPVCNotFilteredTags)
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithPVCNotFilteredTags)

	// pvc tags show up
	podWithPVCTags := append(commontesting.InstanceTags, "persistentvolumeclaim:www-web-2", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithPVCTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithPVCTags)

	// ephemeral volume tags show up
	podWithEphemeralTags := append(commontesting.InstanceTags, "persistentvolumeclaim:web-2-ephemeralvolume", "namespace:default", "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default")

	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.capacity_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.used_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.available_bytes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_used", podWithEphemeralTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.volume.stats.inodes_free", podWithEphemeralTags)
}

func (suite *ProviderTestSuite) TestSendAlwaysCounter() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics_1_21.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// expected counters show up with flushFirstValue=false on first run
	suite.mockSender.AssertMonotonicCount(suite.T(), "MonotonicCountWithFlushFirstValue", common.KubeletMetricsPrefix+"kubelet.evictions", 3, "", append(commontesting.InstanceTags, "eviction_signal:allocatableMemory.available"), false)
	suite.mockSender.AssertMonotonicCount(suite.T(), "MonotonicCountWithFlushFirstValue", common.KubeletMetricsPrefix+"kubelet.evictions", 3, "", append(commontesting.InstanceTags, "eviction_signal:memory.available"), false)
	suite.mockSender.AssertMonotonicCount(suite.T(), "MonotonicCountWithFlushFirstValue", common.KubeletMetricsPrefix+"kubelet.pleg.discard_events", 0, "", commontesting.InstanceTags, false)
}

func (suite *ProviderTestSuite) TestFirstRunTracking() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics_1_21.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	// Verify initial state
	assert.True(suite.T(), suite.provider.firstRun, "Provider should start with firstRun = true")

	// First run - flushFirstValue should be false (!firstRun = !true = false)
	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// Verify firstRun is now false after successful call
	assert.False(suite.T(), suite.provider.firstRun, "Provider should have firstRun = false after successful call")

	// Reset mock sender for second run
	suite.mockSender.ResetCalls()

	// Second run - flushFirstValue should be true (!firstRun = !false = true)
	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// Verify firstRun remains false
	assert.False(suite.T(), suite.provider.firstRun, "Provider should keep firstRun = false after subsequent successful calls")
}

func (suite *ProviderTestSuite) TestSendAlwaysCounterSubsequentRuns() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics_1_21.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	// First run to set firstRun = false
	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// Reset mock calls for second run
	suite.mockSender.ResetCalls()

	// Second run - flushFirstValue should be true (!firstRun = !false = true)
	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// expected counters show up with flushFirstValue=true on subsequent runs
	suite.mockSender.AssertMonotonicCount(suite.T(), "MonotonicCountWithFlushFirstValue", common.KubeletMetricsPrefix+"kubelet.evictions", 3, "", append(commontesting.InstanceTags, "eviction_signal:allocatableMemory.available"), true)
	suite.mockSender.AssertMonotonicCount(suite.T(), "MonotonicCountWithFlushFirstValue", common.KubeletMetricsPrefix+"kubelet.evictions", 3, "", append(commontesting.InstanceTags, "eviction_signal:memory.available"), true)
	suite.mockSender.AssertMonotonicCount(suite.T(), "MonotonicCountWithFlushFirstValue", common.KubeletMetricsPrefix+"kubelet.pleg.discard_events", 0, "", commontesting.InstanceTags, true)
}

func (suite *ProviderTestSuite) TestFirstRunRemainsOnError() {
	// Create a mock that returns an error
	testError := errors.New("kubelet connection failed")
	errorResponse := commontesting.NewEndpointResponse(
		"", 500, testError)

	kubeletMock, err := commontesting.CreateKubeletMock(errorResponse, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	// Verify initial state
	assert.True(suite.T(), suite.provider.firstRun, "Provider should start with firstRun = true")

	// Call should fail
	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	assert.Error(suite.T(), err, "Provider.Provide should return error on kubelet failure")

	// Verify firstRun remains true after failed call
	assert.True(suite.T(), suite.provider.firstRun, "Provider should keep firstRun = true after failed call")
}

func (suite *ProviderTestSuite) TestKubeletContainerLogFilesystemUsedBytes() {
	// Get around floating point conversion issues during AssertCalled
	expectedCalled, _ := strconv.ParseFloat("24576", 64)
	expectedNotCalled, _ := strconv.ParseFloat("5227072", 64)

	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics_1_21.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// container id has tags, so container tags show up
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.container.log_filesystem.used_bytes", 5242822656, "", append(commontesting.InstanceTags, "kube_container_name:datadog-agent"))
	// container id not found in tagger, so no container tags show up
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.container.log_filesystem.used_bytes", expectedCalled, "", commontesting.InstanceTags)
	// container is excluded, so no metric should be emitted at all
	suite.mockSender.AssertNotCalled(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.container.log_filesystem.used_bytes", expectedNotCalled, "", commontesting.InstanceTags)
}

func (suite *ProviderTestSuite) TestRestClientLatency() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics_1_21.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// url is parsed
	// note: there are so many metric points generated for this metric based on the input data, we are just going to focus on one
	expectedTags := append(commontesting.InstanceTags, "url:/api/v1/namespaces/{namespace}/configmaps", "verb:GET")
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
	response := commontesting.NewEndpointResponse(
		"../../testdata/kubelet_metrics_1_21.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// upper_bound tag is transformed for buckets
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.network_plugin.latency.count", 14, "", append(commontesting.InstanceTags, "operation_type:get_pod_network_status", "upper_bound:5000.000000"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.start.duration.count", 30, "", append(commontesting.InstanceTags, "upper_bound:5000.000000"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.worker.start.duration.count", 30, "", append(commontesting.InstanceTags, "upper_bound:5000.000000"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.runtime.operations.duration.count", 177, "", append(commontesting.InstanceTags, "operation_type:container_status", "upper_bound:5000.000000"))

	// value is transformed for sum
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.network_plugin.latency.sum", 1.1268392169999992e+06, "", append(commontesting.InstanceTags, "operation_type:get_pod_network_status"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.start.duration.sum", 202368874.00600008, "", commontesting.InstanceTags)
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.pod.worker.start.duration.sum", 26680.296, "", commontesting.InstanceTags)
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"kubelet.runtime.operations.duration.sum", 1204396.2709999991, "", append(commontesting.InstanceTags, "operation_type:container_status"))
}
