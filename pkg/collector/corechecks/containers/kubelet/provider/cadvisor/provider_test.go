// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package cadvisor

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	commontesting "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common/testing"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/prometheus"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	endpoint = "/metrics/cadvisor"
)

var (
	metricsWithDeviceTagRate = map[string]string{
		common.KubeletMetricsPrefix + "io.read_bytes":  "/dev/sda",
		common.KubeletMetricsPrefix + "io.write_bytes": "/dev/sda",
	}

	metricsWithDeviceTagGauge = map[string]string{
		common.KubeletMetricsPrefix + "filesystem.usage": "/dev/sda1",
	}

	metricsWithInterfaceTag = map[string]string{
		common.KubeletMetricsPrefix + "network.rx_bytes":   "eth0",
		common.KubeletMetricsPrefix + "network.tx_bytes":   "eth0",
		common.KubeletMetricsPrefix + "network.rx_errors":  "eth0",
		common.KubeletMetricsPrefix + "network.tx_errors":  "eth0",
		common.KubeletMetricsPrefix + "network.rx_dropped": "eth0",
		common.KubeletMetricsPrefix + "network.tx_dropped": "eth0",
	}

	expectedMetricsPrometheus = []string{
		common.KubeletMetricsPrefix + "cpu.usage.total",
		common.KubeletMetricsPrefix + "cpu.load.10s.avg",
		common.KubeletMetricsPrefix + "cpu.system.total",
		common.KubeletMetricsPrefix + "cpu.user.total",
		common.KubeletMetricsPrefix + "cpu.cfs.periods",
		common.KubeletMetricsPrefix + "cpu.cfs.throttled.periods",
		common.KubeletMetricsPrefix + "cpu.cfs.throttled.seconds",
		common.KubeletMetricsPrefix + "network.rx_dropped",
		common.KubeletMetricsPrefix + "network.rx_errors",
		common.KubeletMetricsPrefix + "network.tx_dropped",
		common.KubeletMetricsPrefix + "network.tx_errors",
		common.KubeletMetricsPrefix + "network.rx_bytes",
		common.KubeletMetricsPrefix + "network.tx_bytes",
		common.KubeletMetricsPrefix + "io.write_bytes",
		common.KubeletMetricsPrefix + "io.read_bytes",
		common.KubeletMetricsPrefix + "filesystem.usage",
		common.KubeletMetricsPrefix + "filesystem.usage_pct",
		common.KubeletMetricsPrefix + "memory.limits",
		common.KubeletMetricsPrefix + "memory.usage",
		common.KubeletMetricsPrefix + "memory.usage_pct",
		common.KubeletMetricsPrefix + "memory.sw_limit",
		common.KubeletMetricsPrefix + "memory.sw_in_use",
		common.KubeletMetricsPrefix + "memory.working_set",
		common.KubeletMetricsPrefix + "memory.cache",
		common.KubeletMetricsPrefix + "memory.rss",
		common.KubeletMetricsPrefix + "memory.swap",
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

	fakeTagger := taggerimpl.SetupFakeTagger(suite.T())

	for entity, tags := range commontesting.CommonTags {
		prefix, id, _ := taggercommon.ExtractPrefixAndID(entity)
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

	p, err := NewProvider(
		&containers.Filter{
			Enabled:         true,
			NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
		},
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
			name: "pre 1.16 metrics all show up",
			response: commontesting.NewEndpointResponse(
				"../../testdata/cadvisor_metrics_pre_1_16.txt", 200, nil),
			want: want{
				metrics: expectedMetricsPrometheus,
			},
		},
		{
			// All the metrics reported by this provider require container or pod metadata in the store, and drop the
			// metrics if no matching entry is found. So, there should be no reported metrics if there is no pod data
			// (or, in this case, no matching pod data)
			name: "no matching pod data no metrics show up",
			response: commontesting.NewEndpointResponse(
				"../../testdata/cadvisor_metrics_1_21.txt", 200, nil),
			want: want{
				metrics: []string{},
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

			// Provide is called twice so pct metrics are guaranteed to be there
			_ = suite.provider.Provide(kubeletMock, suite.mockSender)
			suite.mockSender.ResetCalls()
			err = suite.provider.Provide(kubeletMock, suite.mockSender)
			if !reflect.DeepEqual(err, tt.want.err) {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}

			commontesting.AssertMetricCallsMatch(t, tt.want.metrics, suite.mockSender)
		})
	}
}

func (suite *ProviderTestSuite) TestPrometheusFiltering() {
	tests := []struct {
		name                string
		response            commontesting.EndpointResponse
		expectedSampleCount int
	}{
		{
			name: "pre 1.16 missing pod_name is excluded",
			response: commontesting.NewEndpointResponse(
				"../../testdata/cadvisor_metrics_pre_1_16.txt", 200, nil),
			expectedSampleCount: 12, // 12 out of 45 total metric points should show up
		},
		{
			name: "post 1.16 missing pod is excluded",
			response: commontesting.NewEndpointResponse(
				"../../testdata/cadvisor_metrics_post_1_16.txt", 200, nil),
			expectedSampleCount: 27, // 27 out of 31 total metric points should show up
		},
	}
	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			suite.SetupTest()
			kubeletMock, err := commontesting.CreateKubeletMock(tt.response, endpoint)
			if err != nil {
				suite.T().Fatalf("error created kubelet mock: %v", err)
			}

			prometheus.ParseMetricsWithFilterFunc = func(data []byte, filter []string) ([]*prom.MetricFamily, error) {
				// We are going to intercept the parsed prometheus metric family data to determine if the configured provider
				// has the expected text blacklist settings by default, and that this functionality still works
				metrics, err := prom.ParseMetricsWithFilter(data, filter)
				var found bool
				for _, metric := range metrics {
					if metric.Name == "container_cpu_usage_seconds_total" {
						found = true
						if len(metric.Samples) != tt.expectedSampleCount {
							t.Errorf("Expected %v samples for metric 'container_cpu_usage_seconds_total', got %v", tt.expectedSampleCount, len(metric.Samples))
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected to find metric 'container_cpu_usage_seconds_total' in parsed output, but it was not there")
				}
				return metrics, err
			}

			err = suite.provider.Provide(kubeletMock, suite.mockSender)
			if err != nil {
				suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
			}
			suite.T().Cleanup(func() {
				// reset the ParseMetricsWithFilterFunc back to what it is supposed to be
				prometheus.ParseMetricsWithFilterFunc = prom.ParseMetricsWithFilter
			})
		})
	}
}

func (suite *ProviderTestSuite) TestIgnoreMetrics() {
	oldIgnore := ignoreMetrics
	ignoreMetrics = []string{"container_network_[Aa-zZ]*_bytes_total"}
	defer suite.T().Cleanup(func() {
		// reset ignoreMetrics back to what it is supposed to be
		ignoreMetrics = oldIgnore
	})
	// since we updated ignoreMetrics, we need to recreate the provider
	suite.provider, _ = NewProvider(suite.provider.filter, suite.provider.Config, suite.provider.store, suite.provider.podUtils, suite.tagger)

	response := commontesting.NewEndpointResponse(
		"../../testdata/cadvisor_metrics_pre_1_16.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// this metric is not filtered out by the regex
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.tx_dropped", commontesting.InstanceTags)
	// these metrics are disabled
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.tx_bytes", commontesting.InstanceTags)
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", commontesting.InstanceTags)
}

func (suite *ProviderTestSuite) TestPrometheusNetSummed() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/cadvisor_metrics_pre_1_16.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// Make sure we submit the summed rates correctly for pods:
	// - dd-agent-q6hpw has two interfaces, we need to sum (1.2638051777 + 2.2638051777) * 10**10 = 35276103554
	// - fluentd-gcp-v2.0.10-9q9t4 has one interface only, we submit 5.8107648 * 10**07 = 58107648
	suite.mockSender.AssertMetric(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 35276103554.0, "", append(commontesting.InstanceTags, "pod_name:dd-agent-q6hpw", "interface:eth0"))
	suite.mockSender.AssertMetric(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 58107648.0, "", append(commontesting.InstanceTags, "pod_name:fluentd-gcp-v2.0.10-9q9t4", "interface:eth0"))

	// Make sure none of the following "bad cases" are submitted:
	// Make sure the per-interface metrics are not submitted
	suite.mockSender.AssertNotCalled(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 12638051777.0, "", append(commontesting.InstanceTags, "pod_name:dd-agent-q6hpw"))
	suite.mockSender.AssertNotCalled(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 22638051777.0, "", append(commontesting.InstanceTags, "pod_name:dd-agent-q6hpw"))
	// Make sure hostNetwork pod metrics are not submitted, test with and without sum to be sure
	suite.mockSender.AssertNotCalled(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 4917138204.0+698882782.0, "", append(commontesting.InstanceTags, "pod_name:kube-proxy-gke-haissam-default-pool-be5066f1-wnvn"))
	suite.mockSender.AssertNotCalled(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 4917138204.0, "", append(commontesting.InstanceTags, "pod_name:kube-proxy-gke-haissam-default-pool-be5066f1-wnvn"))
	suite.mockSender.AssertNotCalled(suite.T(), "Rate", common.KubeletMetricsPrefix+"network.rx_bytes", 698882782.0, "", append(commontesting.InstanceTags, "pod_name:kube-proxy-gke-haissam-default-pool-be5066f1-wnvn"))
}

func (suite *ProviderTestSuite) TestStaticPods() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/cadvisor_metrics_pre_1_16.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	// Test that we get metrics for this static pod
	suite.mockSender.AssertMetric(suite.T(), "Rate", common.KubeletMetricsPrefix+"cpu.user.total", 109.76, "", append(commontesting.InstanceTags, "kube_container_name:kube-proxy", "pod_name:kube-proxy-gke-haissam-default-pool-be5066f1-wnvn"))
}

func (suite *ProviderTestSuite) TestAddLabelsToTags() {
	response := commontesting.NewEndpointResponse(
		"../../testdata/cadvisor_metrics_pre_1_16.txt", 200, nil)

	kubeletMock, err := commontesting.CreateKubeletMock(response, endpoint)
	if err != nil {
		suite.T().Fatalf("error created kubelet mock: %v", err)
	}

	err = suite.provider.Provide(kubeletMock, suite.mockSender)
	if err != nil {
		suite.T().Fatalf("unexpected error returned by call to provider.Provide: %v", err)
	}

	for k, v := range metricsWithDeviceTagRate {
		tag := "device:" + v
		suite.mockSender.AssertMetricTaggedWith(suite.T(), "Rate", k, append(commontesting.InstanceTags, tag))
	}

	for k, v := range metricsWithDeviceTagGauge {
		tag := "device:" + v
		suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", k, append(commontesting.InstanceTags, tag))
	}

	for k, v := range metricsWithInterfaceTag {
		tag := "interface:" + v
		suite.mockSender.AssertMetricTaggedWith(suite.T(), "Rate", k, append(commontesting.InstanceTags, tag))
	}
}
