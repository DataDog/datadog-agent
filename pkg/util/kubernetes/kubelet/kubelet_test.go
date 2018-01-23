// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// dummyKubelet allows tests to mock a kubelet's responses
type dummyKubelet struct {
	Requests chan *http.Request
	PodsBody []byte
}

func createFakePath() string {
	fakePath := "/tmp"
	for {
		fakePath = filepath.Join(fakePath, "layer")
		_, err := os.Stat(fakePath)
		if err != nil && os.IsNotExist(err) {
			return fakePath
		}
	}
}

func newDummyKubelet(podListJSONPath string) (*dummyKubelet, error) {
	if podListJSONPath == "" {
		kubelet := &dummyKubelet{Requests: make(chan *http.Request, 3)}
		return kubelet, nil
	}

	podList, err := ioutil.ReadFile(podListJSONPath)
	if err != nil {
		return nil, err
	}
	kubelet := &dummyKubelet{
		Requests: make(chan *http.Request, 3),
		PodsBody: podList,
	}
	return kubelet, nil
}

func (d *dummyKubelet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("dummyKubelet received %s on %s", r.Method, r.URL.Path)
	d.Requests <- r
	switch r.URL.Path {
	case "/healthz":
		w.Write([]byte("ok"))

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
	return ts, kubeletPort, nil
}

func (d *dummyKubelet) StartTLS() (*httptest.Server, int, error) {
	ts := httptest.NewTLSServer(d)
	return d.parsePort(ts)
}

func (d *dummyKubelet) Start() (*httptest.Server, int, error) {
	ts := httptest.NewServer(d)
	return d.parsePort(ts)
}

type KubeletTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *KubeletTestSuite) SetupTest() {
	ResetGlobalKubeUtil()
}

func (suite *KubeletTestSuite) TestLocateKubeletHTTP() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")
	config.Datadog.Set("kubernetes_http_kubelet_port", kubeletPort)
	config.Datadog.Set("kubernetes_https_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_tls_verify", false)
	config.Datadog.Set("kubelet_auth_token_path", "")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), ku)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), "GET", r.Method)
		require.Equal(suite.T(), "/pods", r.URL.Path)
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetLocalPodList() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.Set("kubernetes_kubelet_host", "localhost")
	config.Datadog.Set("kubernetes_http_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_tls_verify", false)
	config.Datadog.Set("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	pods, err := kubeutil.GetLocalPodList()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pods)
	require.Len(suite.T(), pods, 4)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetNodeInfo() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.Set("kubernetes_kubelet_host", "localhost")
	config.Datadog.Set("kubernetes_http_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_tls_verify", false)
	config.Datadog.Set("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	ip, name, err := kubeutil.GetNodeInfo()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), ip, "10.132.0.9")
	require.Equal(suite.T(), name, "hostname")

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetPodForContainerID() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.Set("kubernetes_kubelet_host", "localhost")
	config.Datadog.Set("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	// Empty container ID
	pod, err := kubeutil.GetPodForContainerID("")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), pod)
	require.NotNil(suite.T(), err)
	require.Contains(suite.T(), err.Error(), "containerID is empty")

	// Invalid container ID
	pod, err = kubeutil.GetPodForContainerID("invalid")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), pod)
	require.NotNil(suite.T(), err)
	require.Contains(suite.T(), err.Error(), "container invalid not found in podlist")

	// Valid container ID
	pod, err = kubeutil.GetPodForContainerID("docker://1ce04128b3cccd7de0ae383516c28e0fe35cbb093195a72661723bdc06934840")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pod)
	require.Equal(suite.T(), pod.Metadata.Name, "kube-dns-1829567597-2xtct")
}

func (suite *KubeletTestSuite) TestKubeletInitFailOnToken() {
	fakePath := createFakePath()
	config.Datadog.Set("kubelet_auth_token_path", fakePath)
	config.Datadog.Set("kubelet_tls_verify", false)

	ku := newKubeUtil()
	err := ku.init()
	expectedErr := fmt.Errorf("could not read token from %s: open %s: no such file or directory", fakePath, fakePath)
	assert.Equal(suite.T(), expectedErr, err)
}

func (suite *KubeletTestSuite) TestKubeletInitTokenHttps() {
	// with a token, without certs on HTTPS insecure
	k, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.StartTLS()
	require.Nil(suite.T(), err)
	defer s.Close()

	config.Datadog.Set("kubernetes_https_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_auth_token_path", "./testdata/fakeBearerToken")
	config.Datadog.Set("kubelet_tls_verify", false)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), fmt.Sprintf("https://127.0.0.1:%d", kubeletPort), ku.kubeletApiEndpoint)
	assert.Equal(suite.T(), "bearer fakeBearerToken", ku.kubeletApiRequestHeaders.Get("Authorization"))
	assert.True(suite.T(), ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))
	assert.Equal(suite.T(), 200, code)
}

func (suite *KubeletTestSuite) TestKubeletInitTokenHttp() {
	// with an unused token, without certs on HTTP
	k, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.Start()
	require.Nil(suite.T(), err)
	defer s.Close()

	config.Datadog.Set("kubernetes_http_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_auth_token_path", "./testdata/unusedBearerToken")
	config.Datadog.Set("kubelet_tls_verify", false)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), fmt.Sprintf("http://127.0.0.1:%d", kubeletPort), ku.kubeletApiEndpoint)
	assert.Equal(suite.T(), "", ku.kubeletApiRequestHeaders.Get("Authorization"))
	assert.True(suite.T(), ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))
	assert.Equal(suite.T(), 200, code)
}

func (suite *KubeletTestSuite) TestKubeletInitHttp() {
	// without token, without certs on HTTP
	k, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.Start()
	require.Nil(suite.T(), err)
	defer s.Close()

	config.Datadog.Set("kubernetes_http_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_auth_token_path", "")
	config.Datadog.Set("kubelet_tls_verify", false)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), fmt.Sprintf("http://127.0.0.1:%d", kubeletPort), ku.kubeletApiEndpoint)
	assert.Equal(suite.T(), "", ku.kubeletApiRequestHeaders.Get("Authorization"))
	assert.True(suite.T(), ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))
	assert.Equal(suite.T(), 200, code)
}

func TestKubeletTestSuite(t *testing.T) {
	suite.Run(t, new(KubeletTestSuite))
}
