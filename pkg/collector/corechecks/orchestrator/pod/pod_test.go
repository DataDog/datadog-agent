// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubelet && orchestrator

package pod

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	testHostName = "test-host"
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

type PodTestSuite struct {
	suite.Suite
	check        *Check
	dummyKubelet *dummyKubelet
	testServer   *httptest.Server
	mockSender   *mocksender.MockSender
	kubeUtil     kubelet.KubeUtilInterface
}

func (suite *PodTestSuite) SetupTest() {
	kubelet.ResetGlobalKubeUtil()
	kubelet.ResetCache()

	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)

	mockConfig := config.Mock(nil)

	mockSender := mocksender.NewMockSender(checkid.ID(suite.T().Name()))
	mockSender.SetupAcceptAll()
	suite.mockSender = mockSender

	suite.dummyKubelet = newDummyKubelet()
	ts, kubeletPort, err := suite.dummyKubelet.Start()
	require.NoError(suite.T(), err)
	suite.testServer = ts

	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("orchestrator_explorer.enabled", true)
	mockConfig.Set("orchestrator_explorer.run_on_node_agent", false)
	mockConfig.Set("orchestrator_explorer.manifest_collection.enabled", true)

	kubeutil, _ := kubelet.GetKubeUtilWithRetrier()
	require.NotNil(suite.T(), kubeutil)
	suite.kubeUtil = kubeutil

	orchConfig := oconfig.NewDefaultOrchestratorConfig()
	orchConfig.CoreCheck = true
	require.NoError(suite.T(), err)
	suite.check = &Check{
		sender:    mockSender,
		processor: processors.NewProcessor(new(k8sProcessors.PodHandlers)),
		hostName:  testHostName,
		config:    orchConfig,
	}
}

func (suite *PodTestSuite) TearDownTest() {
	suite.testServer.Close()
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(PodTestSuite))
}

func (suite *PodTestSuite) TestTransformPods() {
	err := suite.dummyKubelet.loadPodList("../testdata/pods.json")
	require.NoError(suite.T(), err)
	suite.check.Run()
	// TODO require no error when enabled
	// err = suite.check.Run()
	// require.NoError(suite.T(), err)
	// suite.mockSender.AssertNumberOfCalls(suite.T(), "OrchestratorMetadata", 1)
}
