// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pod

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	commontesting "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common/testing"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dummyKubelet allows tests to mock a kubelet's responses
type dummyKubelet struct {
	sync.Mutex
	PodsBody []byte
}

func newDummyKubelet() *dummyKubelet {
	return &dummyKubelet{}
}

func (d *dummyKubelet) loadPodList(podListJSONPath string) error {
	d.Lock()
	defer d.Unlock()
	podList, err := os.ReadFile(podListJSONPath)
	if err != nil {
		return err
	}
	d.PodsBody = podList
	return nil
}

func (d *dummyKubelet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.Lock()
	defer d.Unlock()
	switch r.URL.Path {
	case "/pods":
		if d.PodsBody == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s, err := w.Write(d.PodsBody)
		log.Debugf("dummyKubelet wrote %d bytes, err: %v", s, err)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *dummyKubelet) parsePort(ts *httptest.Server) (*httptest.Server, int, error) {
	kubeletURL, err := url.Parse(ts.URL)
	if err != nil {
		return nil, 0, err
	}
	kubeletPort, err := strconv.Atoi(kubeletURL.Port())
	if err != nil {
		return nil, 0, err
	}
	log.Debugf("Starting on port %d", kubeletPort)
	return ts, kubeletPort, nil
}

func (d *dummyKubelet) Start() (*httptest.Server, int, error) {
	ts := httptest.NewServer(d)
	return d.parsePort(ts)
}

type ProviderTestSuite struct {
	suite.Suite
	provider     *Provider
	dummyKubelet *dummyKubelet
	testServer   *httptest.Server
	mockSender   *mocksender.MockSender
	kubeUtil     kubelet.KubeUtilInterface
}

func (suite *ProviderTestSuite) SetupTest() {
	kubelet.ResetGlobalKubeUtil()
	kubelet.ResetCache()

	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)

	mockConfig := config.Mock(nil)

	mockSender := mocksender.NewMockSender(checkid.ID(suite.T().Name()))
	mockSender.SetupAcceptAll()
	suite.mockSender = mockSender

	fakeTagger := local.NewFakeTagger()
	for entity, tags := range commontesting.CommonTags {
		fakeTagger.SetTags(entity, "foo", tags, nil, nil, nil)
	}
	tagger.SetDefaultTagger(fakeTagger)

	suite.dummyKubelet = newDummyKubelet()
	ts, kubeletPort, err := suite.dummyKubelet.Start()
	require.Nil(suite.T(), err)
	suite.testServer = ts

	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	kubeutil, _ := kubelet.GetKubeUtilWithRetrier()
	require.NotNil(suite.T(), kubeutil)
	suite.kubeUtil = kubeutil

	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags: []string{"instance_tag:something"},
		},
	}

	suite.provider = &Provider{
		config: config,
		filter: &containers.Filter{
			Enabled:         true,
			NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
		},
		podUtils: common.NewPodUtils(),
	}
}

func (suite *ProviderTestSuite) TearDownTest() {
	suite.testServer.Close()
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}

func (suite *ProviderTestSuite) TestTransformRunningPods() {
	err := suite.dummyKubelet.loadPodList("../../testdata/pods.json")
	require.Nil(suite.T(), err)
	config := suite.provider.config

	err = suite.provider.Provide(suite.kubeUtil, suite.mockSender)
	require.Nil(suite.T(), err)

	suite.mockSender.AssertNumberOfCalls(suite.T(), "Gauge", 30)

	// 1) pod running metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 2, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 1, "", append(config.Tags, "kube_container_name:datadog-agent"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 1, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", 2, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))

	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:fluentd-gcp-v2.0.10-9q9t4"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:fluentd-gcp-v2.0.10-p13r3"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "pod_name:datadog-agent-jbm2k"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default", "persistentvolumeclaim:www-web-2", "persistentvolumeclaim:www2-web-3", "pod_phase:running"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", 1, "", append(config.Tags, "kube_namespace:default", "kube_service:nginx", "kube_stateful_set:web", "namespace:default", "persistentvolumeclaim:www-web-2", "pod_phase:running"))

	// make sure that non-running container/pods are not sent
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.running", append(config.Tags, "pod_name:dd-agent-q6hpw"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.running", append(config.Tags, "pod_name:dd-agent-q6hpw"))

	// 2) container spec metrics
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.1, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.1, "", append(config.Tags, "kube_container_name:datadog-agent"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.1, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", 0.25, "", append(config.Tags, "kube_container_name:datadog-agent"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 209715200, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 134217728, "", append(config.Tags, "kube_container_name:datadog-agent"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 314572800, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 536870912, "", append(config.Tags, "kube_container_name:datadog-agent"))

	// make sure that resource metrics are not sent for non-running pods
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", append(config.Tags, "pod_name:pi-kff76"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", append(config.Tags, "pod_name:pi-kff76"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", append(config.Tags, "pod_name:pi-kff76"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", append(config.Tags, "pod_name:pi-kff76"))

	// 3) container status metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"pods.expired", 1, "", config.Tags)
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10"))
	// should be called twice
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:datadog-agent"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))
}

func (suite *ProviderTestSuite) TestTransformCrashedPods() {
	err := suite.dummyKubelet.loadPodList("../../testdata/pods_crashed.json")
	require.Nil(suite.T(), err)
	config := suite.provider.config

	err = suite.provider.Provide(suite.kubeUtil, suite.mockSender)
	require.Nil(suite.T(), err)

	// container state metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.last_state.terminated", 1, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10", "reason:oomkilled"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.waiting", 1, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10", "reason:crashloopbackoff"))

	// container restarts metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 1, "", append(config.Tags, "kube_container_name:fluentd-gcp", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:prometheus-to-sd-exporter", "kube_deployment:fluentd-gcp-v2.0.10"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "kube_container_name:datadog-agent"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.restarts", 0, "", append(config.Tags, "pod_name:demo-app-success-c485bc67b-klj45"))

	// ensure that TransientReason is filtered from being reported
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.waiting", append(config.Tags, "reason:transientreason"))
	suite.mockSender.AssertMetricNotTaggedWith(suite.T(), "Gauge", common.KubeletMetricsPrefix+"containers.state.terminated", append(config.Tags, "reason:transientreason"))

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
	err := suite.dummyKubelet.loadPodList("../../testdata/pods_requests_limits.json")
	require.Nil(suite.T(), err)
	config := suite.provider.config

	err = suite.provider.Provide(suite.kubeUtil, suite.mockSender)
	require.Nil(suite.T(), err)

	// container resource metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 1073741824.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral-storage.requests", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 1073741824.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral-storage.limits", 2147483648.0, "", append(config.Tags, "pod_name:cassandra-0"))
}
