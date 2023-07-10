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
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	mockSender := mocksender.NewMockSender(check.ID(suite.T().Name()))
	mockSender.SetupAcceptAll()
	suite.mockSender = mockSender

	fakeTagger := local.NewFakeTagger()
	for entity, tags := range commonTags {
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

	config := &common.KubeletConfig{Tags: []string{"instance_tag:something"}}

	suite.provider = &Provider{
		config: config,
		filter: &containers.Filter{
			Enabled:         true,
			NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
		},
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

	pods, err := suite.provider.Collect(suite.kubeUtil)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pods)

	err = suite.provider.Transform(pods, suite.mockSender)
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

	pods, err := suite.provider.Collect(suite.kubeUtil)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pods)

	err = suite.provider.Transform(pods, suite.mockSender)
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

	pods, err := suite.provider.Collect(suite.kubeUtil)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pods)

	err = suite.provider.Transform(pods, suite.mockSender)
	require.Nil(suite.T(), err)

	// container resource metrics
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.requests", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.requests", 1073741824.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral-storage.requests", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"cpu.limits", 0.5, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"memory.limits", 1073741824.0, "", append(config.Tags, "pod_name:cassandra-0"))
	suite.mockSender.AssertMetric(suite.T(), "Gauge", common.KubeletMetricsPrefix+"ephemeral-storage.limits", 2147483648.0, "", append(config.Tags, "pod_name:cassandra-0"))
}
