// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pod

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	commontesting "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common/testing"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type ProviderTestSuite struct {
	suite.Suite
	provider   *Provider
	mockSender *mocksender.MockSender
	tagger     tagger.Component
	fakeTagger taggermock.Mock
}

func (suite *ProviderTestSuite) SetupTest() {
	mockConfig := configmock.New(suite.T())

	mockSender := mocksender.NewMockSender(suite.T(), checkid.ID(suite.T().Name()))
	mockSender.SetupAcceptAll()
	suite.mockSender = mockSender

	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())

	for entity, tags := range commontesting.CommonTags {
		prefix, id, _ := taggertypes.ExtractPrefixAndID(entity)
		entityID := taggertypes.NewEntityID(prefix, id)
		fakeTagger.SetTags(entityID, "foo", tags, nil, nil, nil)
	}

	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags:    []string{"instance_tag:something"},
			Timeout: 10,
		},
	}

	suite.tagger = fakeTagger
	suite.fakeTagger = fakeTagger

	// The workloadmeta collectors live in an "internal" package, so we can't
	// import them here. That means we can’t reuse the pod parsing logic in the
	// kubelet collector to read the test file and populate workloadmeta.
	// So instead of that, we're going to configure workloadmeta with the
	// kubelet collector.
	env.SetFeatures(suite.T(), env.Kubernetes) // Required to enable the "kubelet" collector
	wmeta := fxutil.Test[workloadmetamock.Mock](suite.T(), fx.Options(
		core.MockBundle(),
		hostnameimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		workloadfilterfxmock.MockModule(),
		// GetCatalog() returns all collectors but only the kubelet one will
		// be active, thanks to the SetFeatures call above
		wmcatalog.GetCatalog(),
		fx.Provide(func() ipc.Component { return ipcmock.New(suite.T()) }),
	))

	mockConfig.SetInTest("container_exclude", "name:agent-excluded")
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(suite.T())

	suite.provider = NewProvider(mockFilterStore, wmeta, config, common.NewPodUtils(fakeTagger), fakeTagger, nil)
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}

func (suite *ProviderTestSuite) TestTransformRunningPods() {
	config := suite.provider.config

	testDataFile := "../../testdata/pods.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)

	suite.mockSender.AssertNumberOfCalls(suite.T(), "Gauge", 36)

	// 1) pod running metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 2, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 1, "", append(config.Tags, "kube_container_name:datadog-agent", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 1, "", append(config.Tags, "kube_container_name:running-init", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 1, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 2, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))

	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:fluentd-gcp-v2.0.10-9q9t4"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:fluentd-gcp-v2.0.10-p13r3"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:datadog-agent-jbm2k"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default", "persistentvolumeclaim:www-web-2", "persistentvolumeclaim:www2-web-3", "pod_phase:running"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default", "persistentvolumeclaim:www-web-2", "pod_phase:running"))

	// make sure that non-running container/pods are not sent
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", append(config.Tags, "pod_name:dd-agent-q6hpw"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", append(config.Tags, "pod_name:dd-agent-q6hpw", "kube_namespace:default"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))

	// 2) container spec metrics
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.1, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.1, "", append(config.Tags, "kube_container_name:datadog-agent", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.05, "", append(config.Tags, "kube_container_name:running-init", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.1, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", 0.25, "", append(config.Tags, "kube_container_name:datadog-agent", "kube_namespace:default"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 209715200, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 134217728, "", append(config.Tags, "kube_container_name:datadog-agent", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 104857600, "", append(config.Tags, "kube_container_name:running-init", "kube_namespace:default"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 314572800, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 536870912, "", append(config.Tags, "kube_container_name:datadog-agent", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 157286400, "", append(config.Tags, "kube_container_name:running-init", "kube_namespace:default"))

	// make sure that resource metrics are not sent for non-running pods
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", append(config.Tags, "pod_name:pi-kff76"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", append(config.Tags, "pod_name:pi-kff76"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", append(config.Tags, "pod_name:pi-kff76"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", append(config.Tags, "pod_name:pi-kff76"))

	// make sure that resource metrics are not sent from completed init containers
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))

	// 3) container status metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.expired", 1, "", config.Tags)
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:datadog-agent", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:running-init", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45", "kube_namespace:default"))
}

func (suite *ProviderTestSuite) TestTransformCrashedPods() {
	config := suite.provider.config

	testDataFile := "../../testdata/pods_crashed.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)

	// container state metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.last_state.terminated", 1, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "reason:oomkilled"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.waiting", 1, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10", "reason:crashloopbackoff"))

	// container restarts metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 1, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:datadog-agent"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))

	// ensure that TransientReason is filtered from being reported
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.waiting", append(config.Tags, "reason:transientreason"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.terminated", append(config.Tags, "reason:transientreason"))

	// ensure that completed init containers do not report a state metric
	// here, the "reason:completed" tag would be present if we whitelisted this reason. If this line fails,
	// just know that this will emit another data point for every init container, and do you really want to be doing that?
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.terminated", append(config.Tags, "kube_container_name:init", "kube_deployment:fluentd-gcp-v2.0.10"))

	// ensure that all state metrics are emitted with the "reason" tag
	for _, call := range suite.mockSender.Calls {
		if call.Method != "Gauge" || !strings.Contains(call.Arguments[0].(string), "containers.state") {
			continue
		}
		hasReasonTag := false
		for _, tag := range call.Arguments[3].([]string) {
			if strings.HasPrefix(tag, "reason:") {
				hasReasonTag = true
				break
			}
		}

		assert.True(suite.T(), hasReasonTag, "expected metric to be emitted with reason tag: %v", call.Arguments)
	}
}

func (suite *ProviderTestSuite) TestTransformPodsRequestsLimits() {
	config := suite.provider.config

	testDataFile := "../../testdata/pods_requests_limits.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)

	// container resource metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 1073741824.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral-storage.requests", 524288000.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 1073741824.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral-storage.limits", 2147483648.0, "", append(config.Tags, "pod_name:cassandra-0"))

	// pod resource metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.cpu.requests", 0.75, "", append(config.Tags, "pod_name:cassandra-0", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.memory.requests", 1610612736.0, "", append(config.Tags, "pod_name:cassandra-0", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.cpu.limits", 1.0, "", append(config.Tags, "pod_name:cassandra-0", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.memory.limits", 2147483648.0, "", append(config.Tags, "pod_name:cassandra-0", "kube_namespace:default"))
}

// TestTransformPodsInPlaceResize verifies that requests/limits metrics
// reflect containerStatuses[].resources (in-place vertical scaling) when
// set, and fall back to the spec for keys status does not report.
func (suite *ProviderTestSuite) TestTransformPodsInPlaceResize() {
	config := suite.provider.config

	testDataFile := "../../testdata/pods_in_place_resize.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)

	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.2, "", append(config.Tags, "kube_container_name:resized-container", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 536870912.0, "", append(config.Tags, "kube_container_name:resized-container", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", 0.2, "", append(config.Tags, "kube_container_name:resized-container", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 536870912.0, "", append(config.Tags, "kube_container_name:resized-container", "kube_namespace:default"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"nvidia.com/gpu.requests", 1.0, "", append(config.Tags, "kube_container_name:resized-container", "kube_namespace:default"))
}

func (suite *ProviderTestSuite) TestNoMetricNoKubeletData() {
	config := suite.provider.config

	testDataFile := "../../testdata/pod_list_with_no_kube_tags.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)
	// ensure that metrics are not emitted when there are no kubelet tags
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter-no-namespace", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45-no-namespace"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", append(config.Tags, "kube_container_name:fluentd-gcp-no-namespace", "kube_deployment:fluentd-gcp-v2.0.10"))
}

func (suite *ProviderTestSuite) TestNoPodMetricsIfDurationIsNegative() {
	config := suite.provider.config

	// termination time: 2018-02-14T14:57:17Z
	testDataFile := "../../testdata/pods_termination.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	suite.provider.now = func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2018-02-14T10:57:17Z")
		return t
	}

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)
	// ensure that metrics are not emitted when there are no kubelet tags
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.terminating.duration", config.Tags)
}

func (suite *ProviderTestSuite) TestPodMetricsIfDurationIsPositive() {
	config := suite.provider.config

	// termination time: 2018-02-14T14:57:17Z
	testDataFile := "../../testdata/pods_termination.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	suite.provider.now = func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2018-02-15T14:57:17Z")
		return t
	}

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)
	// ensure that metrics are not emitted when there are no kubelet tags
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.terminating.duration", 86400, "", config.Tags)
}

func (suite *ProviderTestSuite) TestPodResizeMetrics() {
	config := suite.provider.config

	testDataFile := "../../testdata/pods_pending.json"
	err := suite.fillWorkloadmetaStore(testDataFile)
	require.Nil(suite.T(), err)

	err = suite.provider.Provide(nil, suite.mockSender)
	require.Nil(suite.T(), err)

	// ensure that metrics are not emitted when there are no kubelet tags
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.resize.pending", append(config.Tags, "reason:infeasible"))
	suite.mockSender.AssertMetricTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pod.resize.pending", append(config.Tags, "reason:deferred"))
}

func (suite *ProviderTestSuite) fillWorkloadmetaStore(testDataFile string) error {
	data, err := os.ReadFile(testDataFile)
	if err != nil {
		return err
	}

	var podList kubelet.PodList
	if err := kubelet.NewPodUnmarshaller().Unmarshal(data, &podList); err != nil {
		return err
	}

	wmetaEvents := util.ParseKubeletPods(podList.Items, true, suite.provider.store)

	wmetaEvents = append(wmetaEvents, workloadmeta.CollectorEvent{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubeletMetrics{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubeletMetrics,
				ID:   workloadmeta.KubeletMetricsID,
			},
			ExpiredPodCount: podList.ExpiredCount,
		},
	})

	// The Notify function in the mock handles events synchronously
	suite.provider.store.(workloadmetamock.Mock).Notify(wmetaEvents)

	return nil
}

const (
	restartTestPodUID        = "restart-pod-uid-0001"
	restartTestPodName       = "restart-pod"
	restartTestNamespace     = "restart-ns"
	restartTestContainerName = "restart-container"
	restartTestContainerHash = "restartcontainerhash000000000000000000000000000000000000000000000"
)

// restartTestKey mirrors containerRestartStateKey for the restart-count test fixture.
func restartTestKey() string {
	return restartTestNamespace + "/" + restartTestPodUID + "/" + restartTestContainerName
}

func terminatedState(reason string) workloadmeta.KubernetesContainerState {
	return workloadmeta.KubernetesContainerState{
		Terminated: &workloadmeta.KubernetesContainerStateTerminated{Reason: reason},
	}
}

func runningState() workloadmeta.KubernetesContainerState {
	return workloadmeta.KubernetesContainerState{
		Running: &workloadmeta.KubernetesContainerStateRunning{StartedAt: time.Now()},
	}
}

func waitingState(reason string) workloadmeta.KubernetesContainerState {
	return workloadmeta.KubernetesContainerState{
		Waiting: &workloadmeta.KubernetesContainerStateWaiting{Reason: reason},
	}
}

// setRestartTestPod (re)creates the single-container restart-count fixture pod in the
// store with the supplied restart count and states. The pod UID/namespace/name are kept
// stable across runs so the provider's cross-run restart tracking is exercised, and the
// entity is unset first so restart states fully replace (workloadmeta merges append slices).
func (suite *ProviderTestSuite) setRestartTestPod(restartCount int32, state, lastState workloadmeta.KubernetesContainerState) {
	suite.fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, restartTestContainerHash),
		"kubelet",
		[]string{"kube_container_name:" + restartTestContainerName, "kube_namespace:" + restartTestNamespace},
		nil, nil, nil,
	)

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   restartTestPodUID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      restartTestPodName,
			Namespace: restartTestNamespace,
		},
		Phase:      "Running",
		Containers: []workloadmeta.OrchestratorContainer{{ID: restartTestContainerHash, Name: restartTestContainerName}},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{{
			ContainerID:          "containerd://" + restartTestContainerHash,
			Name:                 restartTestContainerName,
			RestartCount:         restartCount,
			State:                state,
			LastTerminationState: lastState,
		}},
	}

	store := suite.provider.store.(workloadmetamock.Mock)
	store.Unset(pod)
	store.Set(pod)
}

func (suite *ProviderTestSuite) unsetRestartTestPod() {
	store := suite.provider.store.(workloadmetamock.Mock)
	store.Unset(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   restartTestPodUID,
		},
	})
}

// runProvide resets the recorded sender calls and invokes Provide once, so assertions
// afterwards only reflect the metrics emitted during that single run.
func (suite *ProviderTestSuite) runProvide() {
	suite.mockSender.ResetCalls()
	require.NoError(suite.T(), suite.provider.Provide(nil, suite.mockSender))
}

func lastStateTerminatedCountMetric() string {
	return common.KubeletMetricsPrefix + "containers.last_state.terminated.count"
}

// TestLastStateTerminatedCountFirstObservationNoCount asserts that a container seen for
// the first time never emits last_state.terminated.count (only a baseline is recorded),
// even when it already reports a terminated LastTerminationState with a non-zero restart count.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountFirstObservationNoCount() {
	suite.setRestartTestPod(3, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()

	suite.mockSender.AssertNotCalled(suite.T(), "Count", lastStateTerminatedCountMetric(),
		mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))

	st, ok := suite.provider.containerRestartCounts[restartTestKey()]
	require.True(suite.T(), ok, "baseline should be recorded on first observation")
	assert.Equal(suite.T(), int32(3), st.count)
}

// TestLastStateTerminatedCountSingleRestart asserts that a single new restart with an
// allowlisted reason emits a Count of 1 tagged with the termination reason.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountSingleRestart() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(1, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()

	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "kube_namespace:"+restartTestNamespace, "reason:oomkilled"))
}

// TestLastStateTerminatedCountMultipleRestartsInInterval asserts that multiple restarts
// observed between two runs are emitted as a single Count carrying the full delta.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountMultipleRestartsInInterval() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(3, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()

	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 3, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
	suite.mockSender.AssertNumberOfCalls(suite.T(), "Count", 1)
}

// TestLastStateTerminatedCountRepeatSameReason asserts that successive single restarts
// with the same reason each emit their own Count of 1 on the same reason series.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountRepeatSameReason() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	for restartCount := int32(1); restartCount <= 3; restartCount++ {
		suite.setRestartTestPod(restartCount, runningState(), terminatedState("OOMKilled"))
		suite.runProvide()
		suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
			append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
		suite.mockSender.AssertNumberOfCalls(suite.T(), "Count", 1)
	}
}

// TestLastStateTerminatedCountReasonChange asserts that when the termination reason changes
// between runs the Count is emitted with the new reason tag.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountReasonChange() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(1, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()
	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))

	suite.setRestartTestPod(2, runningState(), terminatedState("Error"))
	suite.runProvide()
	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:error"))
}

// TestLastStateTerminatedCountNonAllowlistedReasonAdvancesBaseline asserts that a restart
// with a non-allowlisted reason emits no Count but still advances the baseline, so a later
// allowlisted restart is not double-counted.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountNonAllowlistedReasonAdvancesBaseline() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(2, runningState(), terminatedState("Completed"))
	suite.runProvide()
	suite.mockSender.AssertNotCalled(suite.T(), "Count", lastStateTerminatedCountMetric(),
		mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	st, ok := suite.provider.containerRestartCounts[restartTestKey()]
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), int32(2), st.count, "baseline should advance through the skipped restart")

	suite.setRestartTestPod(3, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()
	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
}

// TestLastStateTerminatedCountRebaselineOnDecrease asserts that a decreasing restart count
// (e.g. container recreated) emits no Count and rebaselines to the lower value.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountRebaselineOnDecrease() {
	config := suite.provider.config

	suite.setRestartTestPod(5, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(2, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()
	suite.mockSender.AssertNotCalled(suite.T(), "Count", lastStateTerminatedCountMetric(),
		mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	st, ok := suite.provider.containerRestartCounts[restartTestKey()]
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), int32(2), st.count, "baseline should be rebaselined to the lower value")

	suite.setRestartTestPod(3, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()
	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
}

// TestLastStateTerminatedCountTerminatedNilNoCount characterizes that a positive restart
// delta with a nil terminated LastTerminationState emits no Count, because the last_state
// branch requires Terminated != nil.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountTerminatedNilNoCount() {
	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(2, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.mockSender.AssertNotCalled(suite.T(), "Count", lastStateTerminatedCountMetric(),
		mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

// TestLastStateTerminatedCountEviction asserts that a container absent from a run has its
// restart state evicted, so re-adding it behaves as a first observation again.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountEviction() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()
	_, ok := suite.provider.containerRestartCounts[restartTestKey()]
	require.True(suite.T(), ok, "entry should be recorded while the pod is present")

	suite.unsetRestartTestPod()
	suite.runProvide()
	_, ok = suite.provider.containerRestartCounts[restartTestKey()]
	assert.False(suite.T(), ok, "entry should be evicted once the pod is no longer observed")

	suite.setRestartTestPod(1, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()
	suite.mockSender.AssertNotCalled(suite.T(), "Count", lastStateTerminatedCountMetric(),
		mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))

	suite.setRestartTestPod(2, runningState(), terminatedState("OOMKilled"))
	suite.runProvide()
	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
}

// TestLastStateTerminatedCountDoesNotDisturbExistingGauges asserts that emitting the new
// count leaves the pre-existing containers.restarts / state.waiting / last_state.terminated
// gauges intact.
func (suite *ProviderTestSuite) TestLastStateTerminatedCountDoesNotDisturbExistingGauges() {
	config := suite.provider.config

	suite.setRestartTestPod(0, runningState(), workloadmeta.KubernetesContainerState{})
	suite.runProvide()

	suite.setRestartTestPod(1, waitingState("CrashLoopBackOff"), terminatedState("OOMKilled"))
	suite.runProvide()

	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "kube_namespace:"+restartTestNamespace))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.waiting", 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:crashloopbackoff"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.last_state.terminated", 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
	suite.mockSender.AssertMetric(suite.T(), "Count", lastStateTerminatedCountMetric(), 1, "",
		append(config.Tags, "kube_container_name:"+restartTestContainerName, "reason:oomkilled"))
}
