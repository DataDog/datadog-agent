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

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
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
}

func (suite *ProviderTestSuite) SetupTest() {
	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)

	mockConfig := configmock.New(suite.T())

	mockSender := mocksender.NewMockSender(checkid.ID(suite.T().Name()))
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

	// The workloadmeta collectors live in an "internal" package, so we can't
	// import them here. That means we can’t reuse the pod parsing logic in the
	// kubelet collector to read the test file and populate workloadmeta.
	// So instead of that, we're going to configure workloadmeta with the
	// kubelet collector.
	env.SetFeatures(suite.T(), env.Kubernetes) // Required to enable the "kubelet" collector
	wmeta := fxutil.Test[workloadmetamock.Mock](suite.T(), fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		workloadfilterfxmock.MockModule(),
		// GetCatalog() returns all collectors but only the kubelet one will
		// be active, thanks to the SetFeatures call above
		wmcatalog.GetCatalog(),
		fx.Provide(func() ipc.Component { return ipcmock.New(suite.T()) }),
	))

	mockConfig.SetWithoutSource("container_exclude", "name:agent-excluded")
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(suite.T())

	suite.provider = NewProvider(mockFilterStore, wmeta, config, common.NewPodUtils(fakeTagger), fakeTagger)
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
	if err := jsoniter.Unmarshal(data, &podList); err != nil {
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
