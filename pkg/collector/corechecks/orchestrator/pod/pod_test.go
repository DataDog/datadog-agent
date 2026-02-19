// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubelet && orchestrator && test

package pod

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var testHostName = "test-host"

// dummyKubelet allows tests to mock a kubelet's responses
type dummyKubelet struct {
	sync.Mutex
	PodsBody []byte
}

func newDummyKubelet() *dummyKubelet {
	return &dummyKubelet{}
}

func (d *dummyKubelet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.Lock()
	defer d.Unlock()
	switch r.URL.Path {
	case "/pods":
		podList, err := os.ReadFile("../testdata/pods.json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s, err := w.Write(podList)
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

type fakeSender struct {
	mocksender.MockSender
	pods      []process.MessageBody
	manifests []process.MessageBody
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (s *fakeSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	s.pods = append(s.pods, msgs...)
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (s *fakeSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {
	s.manifests = append(s.manifests, msgs...)
}

type PodTestSuite struct {
	suite.Suite
	check        *Check
	dummyKubelet *dummyKubelet
	testServer   *httptest.Server
	sender       *fakeSender
	kubeUtil     kubelet.KubeUtilInterface
	tagger       taggermock.Mock
}

func (suite *PodTestSuite) SetupSuite() {
	kubelet.ResetGlobalKubeUtil()
	kubelet.ResetCache()
	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)

	suite.dummyKubelet = newDummyKubelet()
	ts, kubeletPort, err := suite.dummyKubelet.Start()
	require.NoError(suite.T(), err)
	suite.testServer = ts

	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.SetInTest("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.SetInTest("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.SetInTest("kubelet_tls_verify", false)
	mockConfig.SetInTest("orchestrator_explorer.enabled", true)
	mockConfig.SetInTest("orchestrator_explorer.manifest_collection.enabled", true)
	mockConfig.SetInTest("kubernetes_pod_labels_as_tags", `{"tier":"dd_tier","component":"dd_component"}`)
	mockConfig.SetInTest("kubernetes_pod_annotations_as_tags", `{"kubernetes.io/config.source":"config_source","kubernetes.io/config.hash":"config_hash"}`)

	kubeutil, _ := kubelet.GetKubeUtilWithRetrier()
	require.NotNil(suite.T(), kubeutil)
	suite.kubeUtil = kubeutil

	sender := &fakeSender{}
	suite.sender = sender

	mockStore := fxutil.Test[workloadmetamock.Mock](suite.T(), fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())
	suite.tagger = fakeTagger

	suite.check = &Check{
		cfg:       mockConfig,
		sender:    sender,
		processor: processors.NewProcessor(k8sProcessors.NewPodHandlers(mockConfig, mockStore, fakeTagger)),
		hostName:  testHostName,
		config:    oconfig.NewDefaultOrchestratorConfig(nil),
		tagger:    fakeTagger,
	}
}

func (suite *PodTestSuite) TearDownSuite() {
	suite.testServer.Close()
}

func TestPodTestSuite(t *testing.T) {
	suite.Run(t, new(PodTestSuite))
}

func (suite *PodTestSuite) TestPodCheck() {
	cacheKey := cache.BuildAgentKey(constants.ClusterIDCacheKey)
	cachedClusterID, found := cache.Cache.Get(cacheKey)
	if !found {
		cache.Cache.Set(cacheKey, strings.Repeat("1", 36), cache.NoExpiration)
	}

	defer func() {
		cache.Cache.Set(cacheKey, cachedClusterID, cache.NoExpiration)
	}()

	err := suite.check.Run()
	require.NoError(suite.T(), err)

	require.Len(suite.T(), suite.sender.pods, 1)
	require.Len(suite.T(), suite.sender.manifests, 1)

	require.Len(suite.T(), suite.sender.pods[0].(*process.CollectorPod).Pods, 10)
	require.Len(suite.T(), suite.sender.manifests[0].(*process.CollectorManifest).Manifests, 10)

	require.Equal(suite.T(),
		sorted(suite.sender.pods[0].(*process.CollectorPod).Tags...),
		sorted("kube_api_version:v1"))
	require.Equal(suite.T(),
		sorted(suite.sender.pods[0].(*process.CollectorPod).Pods[0].Tags...),
		sorted("kube_condition_podscheduled:true", "pod_status:pending",
			"dd_component:kube-proxy", "dd_tier:node",
			"config_hash:260c2b1d43b094af6d6b4ccba082c2db", "config_source:file"))

	require.Equal(suite.T(),
		sorted(suite.sender.manifests[0].(*process.CollectorManifest).Tags...),
		sorted())
	require.Equal(suite.T(),
		sorted(suite.sender.manifests[0].(*process.CollectorManifest).Manifests[0].Tags...),
		sorted("kube_api_version:v1", "kube_condition_podscheduled:true", "pod_status:pending",
			"dd_component:kube-proxy", "dd_tier:node",
			"config_hash:260c2b1d43b094af6d6b4ccba082c2db", "config_source:file"))
}

func sorted(l ...string) []string {
	var s []string
	s = append(s, l...)
	sort.Strings(s)
	return s
}
