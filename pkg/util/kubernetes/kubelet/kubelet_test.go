// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	fakePath = "./testdata/invalidTokenFilePath"
)

// dummyKubelet allows tests to mock a kubelet's responses
type dummyKubelet struct {
	sync.Mutex
	Requests chan *http.Request
	PodsBody []byte

	testingCertificate string
	testingPrivateKey  string
}

func newDummyKubelet(podListJSONPath string) (*dummyKubelet, error) {
	kubelet := &dummyKubelet{Requests: make(chan *http.Request, 3)}
	if podListJSONPath == "" {
		return kubelet, nil
	}
	err := kubelet.loadPodList(podListJSONPath)
	return kubelet, err
}

func (d *dummyKubelet) loadPodList(podListJSONPath string) error {
	d.Lock()
	defer d.Unlock()
	podList, err := ioutil.ReadFile(podListJSONPath)
	if err != nil {
		return err
	}
	d.PodsBody = podList
	return nil
}

func (d *dummyKubelet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.Lock()
	defer d.Unlock()
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
	log.Debugf("Starting on port %d", kubeletPort)
	return ts, kubeletPort, nil
}

func (d *dummyKubelet) getRequest(timeout time.Duration) *http.Request {
	select {
	case r := <-d.Requests:
		return r
	case <-time.After(timeout):
		return nil
	}
}

func (d *dummyKubelet) dropRequests() {
	for {
		select {
		case <-d.Requests:
			continue
		default:
			return
		}
	}
}

func pemBlockForKey(privateKey interface{}) (*pem.Block, error) {
	switch k := privateKey.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil

	default:
		return nil, fmt.Errorf("unrecognized format for privateKey")
	}
}

func (d *dummyKubelet) StartTLS() (*httptest.Server, int, error) {
	ts := httptest.NewTLSServer(d)
	cert := ts.TLS.Certificates
	if len(ts.TLS.Certificates) != 1 {
		return ts, 0, fmt.Errorf("unexpected number of testing certificates: 1 != %d", len(ts.TLS.Certificates))
	}
	certOut, err := ioutil.TempFile("", "kubelet-test-cert-")
	d.testingCertificate = certOut.Name()
	if err != nil {
		return ts, 0, err
	}
	keyOut, err := ioutil.TempFile("", "kubelet-test-key-")
	d.testingPrivateKey = keyOut.Name()
	if err != nil {
		return ts, 0, err
	}
	for _, c := range cert {
		for _, s := range c.Certificate {
			pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: s})
			certOut.Close()
		}
		p, err := pemBlockForKey(c.PrivateKey)
		if err != nil {
			return ts, 0, err
		}
		err = pem.Encode(keyOut, p)
		if err != nil {
			return ts, 0, err
		}
	}
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
	mockConfig := config.NewMock()

	ResetGlobalKubeUtil()
	ResetCache()

	mockConfig.Set("kubelet_client_crt", "")
	mockConfig.Set("kubelet_client_key", "")
	mockConfig.Set("kubelet_client_ca", "")
	mockConfig.Set("kubelet_tls_verify", true)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_wait_on_missing_container", 0)
	mockConfig.Set("kubernetes_kubelet_host", "")
	mockConfig.Set("kubernetes_http_kubelet_port", 10250)
	mockConfig.Set("kubernetes_https_kubelet_port", 10255)
}

func (suite *KubeletTestSuite) TestLocateKubeletHTTP() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), ku)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), "GET", r.Method)
		require.Equal(suite.T(), "/", r.URL.Path)
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}

	require.EqualValues(suite.T(),
		map[string]string{
			"url": fmt.Sprintf("http://127.0.0.1:%d", kubeletPort),
		}, ku.GetRawConnectionInfo())
}

func (suite *KubeletTestSuite) TestGetLocalPodList() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	pods, err := kubeutil.GetLocalPodList()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pods)
	require.Len(suite.T(), pods, 6)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetNodeInfo() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	ip, name, err := kubeutil.GetNodeInfo()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), "192.168.128.141", ip)
	require.Equal(suite.T(), "my-node-name", name)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetHostname() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	hostname, err := kubeutil.GetHostname()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), "my-node-name", hostname)

	// Testing hostname when a cluster name is set
	var testClusterName = "Laika"
	mockConfig.Set("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.Set("cluster_name", "")
	defer clustername.ResetClusterName()

	hostname, err = kubeutil.GetHostname()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), "my-node-name-"+testClusterName, hostname)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestHostnameProvider() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	hostname, err := HostnameProvider("")
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), "my-node-name", hostname)
}

func (suite *KubeletTestSuite) TestPodlistCache() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	kubeutil.GetLocalPodList()
	r := <-kubelet.Requests
	require.Equal(suite.T(), "/pods", r.URL.Path)

	// The request should be cached now
	_, err = kubeutil.GetLocalPodList()
	require.Nil(suite.T(), err)

	select {
	case <-kubelet.Requests:
		assert.FailNow(suite.T(), "podlist request should have been cached")
	default:
		// Cache working as expected
	}

	// test successful cache wipe
	ResetCache()
	_, err = kubeutil.GetLocalPodList()
	require.Nil(suite.T(), err)
	r = <-kubelet.Requests
	require.Equal(suite.T(), "/pods", r.URL.Path)
}

func (suite *KubeletTestSuite) TestGetPodForContainerID() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	// Empty container ID
	pod, err := kubeutil.GetPodForContainerID("")
	<-kubelet.Requests // cache the first /pods request
	require.Nil(suite.T(), pod)
	require.NotNil(suite.T(), err)
	require.Contains(suite.T(), err.Error(), "containerID is empty")

	// Invalid container ID
	pod, err = kubeutil.GetPodForContainerID("invalid")
	// The /pods request is still cached
	require.Nil(suite.T(), pod)
	require.NotNil(suite.T(), err)
	require.True(suite.T(), errors.IsNotFound(err))

	// Valid container ID
	pod, err = kubeutil.GetPodForContainerID("docker://b3e4cd65204e04d1a2d4b7683cae2f59b2075700f033a6b09890bd0d3fecf6b6")
	// The /pods request is still cached
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pod)
	require.Equal(suite.T(), "kube-proxy-rnd5q", pod.Metadata.Name)
}

func (suite *KubeletTestSuite) TestGetPodWaitForContainer() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_empty.json")
	require.NoError(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.NoError(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_wait_on_missing_container", 1)

	kubeutil, err := GetKubeUtil()
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	requests := 0
	var requestsMutex sync.Mutex
	go func() {
		for r := range kubelet.Requests {
			if r.URL.Path != "/pods" {
				continue
			}
			requestsMutex.Lock()
			requests += 1
			requestsMutex.Unlock()
			if requests == 4 { // Initial + cache invalidation + 2 timed retries
				err := kubelet.loadPodList("./testdata/podlist_1.8-2.json")
				assert.NoError(suite.T(), err)
			}
		}
	}()

	// Valid container ID
	pod, err := kubeutil.GetPodForContainerID("docker://b3e4cd65204e04d1a2d4b7683cae2f59b2075700f033a6b09890bd0d3fecf6b6")
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), pod)
	assert.Equal(suite.T(), "kube-proxy-rnd5q", pod.Metadata.Name)
	requestsMutex.Lock()
	assert.Equal(suite.T(), 5, requests)
	requestsMutex.Unlock()
}

func (suite *KubeletTestSuite) TestGetPodDontWaitForContainer() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_empty.json")
	require.NoError(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.NoError(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_wait_on_missing_container", 0)

	kubeutil, err := GetKubeUtil()
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	requests := 0
	var requestsMutex sync.Mutex
	go func() {
		for r := range kubelet.Requests {
			if r.URL.Path == "/pods" {
				requestsMutex.Lock()
				requests += 1
				requestsMutex.Unlock()
			}
		}
	}()

	// We should fail after two requests only (initial + nocache)
	_, err = kubeutil.GetPodForContainerID("docker://b3e4cd65204e04d1a2d4b7683cae2f59b2075700f033a6b09890bd0d3fecf6b6")
	require.Error(suite.T(), err)
	requestsMutex.Lock()
	assert.Equal(suite.T(), 2, requests)
	requestsMutex.Unlock()
}

func (suite *KubeletTestSuite) TestKubeletInitFailOnToken() {
	mockConfig := config.NewMock()

	// without token, with certs on HTTPS insecure
	k, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.StartTLS()
	defer os.Remove(k.testingCertificate)
	defer os.Remove(k.testingPrivateKey)
	require.Nil(suite.T(), err)
	defer s.Close()

	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_auth_token_path", fakePath)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	expectedErr := fmt.Errorf("could not read token from %s: open %s: no such file or directory", fakePath, fakePath)
	assert.Equal(suite.T(), expectedErr, err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), 0, len(ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.Certificates))
}

func (suite *KubeletTestSuite) TestKubeletInitTokenHttps() {
	mockConfig := config.NewMock()

	// with a token, without certs on HTTPS insecure
	k, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.StartTLS()
	defer os.Remove(k.testingCertificate)
	defer os.Remove(k.testingPrivateKey)
	require.Nil(suite.T(), err)
	defer s.Close()

	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_auth_token_path", "./testdata/fakeBearerToken")
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	<-k.Requests // Throwing away first GET

	assert.Equal(suite.T(), fmt.Sprintf("https://127.0.0.1:%d", kubeletPort), ku.kubeletApiEndpoint)
	assert.Equal(suite.T(), "bearer fakeBearerToken", ku.kubeletApiRequestHeaders.Get("Authorization"))
	assert.True(suite.T(), ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))
	assert.Equal(suite.T(), 200, code)
	r := <-k.Requests
	assert.Equal(suite.T(), "bearer fakeBearerToken", r.Header.Get(authorizationHeaderKey))
	assert.Equal(suite.T(), 0, len(ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.Certificates))

	require.EqualValues(suite.T(),
		map[string]string{
			"url":        fmt.Sprintf("https://127.0.0.1:%d", kubeletPort),
			"verify_tls": "false",
			"token":      "fakeBearerToken",
		}, ku.GetRawConnectionInfo())
}

func (suite *KubeletTestSuite) TestKubeletInitHttpsCerts() {
	mockConfig := config.NewMock()

	// with a token, without certs on HTTPS insecure
	k, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.StartTLS()
	defer os.Remove(k.testingCertificate)
	defer os.Remove(k.testingPrivateKey)
	require.Nil(suite.T(), err)
	defer s.Close()

	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_tls_verify", true)
	mockConfig.Set("kubelet_client_crt", k.testingCertificate)
	mockConfig.Set("kubelet_client_key", k.testingPrivateKey)
	mockConfig.Set("kubelet_client_ca", k.testingCertificate)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	<-k.Requests // Throwing away first GET

	assert.Equal(suite.T(), fmt.Sprintf("https://127.0.0.1:%d", kubeletPort), ku.kubeletApiEndpoint)
	assert.False(suite.T(), ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))
	assert.Equal(suite.T(), 200, code)
	r := <-k.Requests
	assert.Equal(suite.T(), "", r.Header.Get(authorizationHeaderKey))
	clientCerts := ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.Certificates
	require.Equal(suite.T(), 1, len(clientCerts))
	assert.Equal(suite.T(), clientCerts, s.TLS.Certificates)

	require.EqualValues(suite.T(),
		map[string]string{
			"url":        fmt.Sprintf("https://127.0.0.1:%d", kubeletPort),
			"verify_tls": "true",
			"client_crt": k.testingCertificate,
			"client_key": k.testingPrivateKey,
			"ca_cert":    k.testingCertificate,
		}, ku.GetRawConnectionInfo())
}

func (suite *KubeletTestSuite) TestKubeletInitTokenHttp() {
	mockConfig := config.NewMock()

	// with an unused token, without certs on HTTP
	k, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.Start()
	require.Nil(suite.T(), err)
	defer s.Close()

	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_auth_token_path", "./testdata/unusedBearerToken")
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku := newKubeUtil()
	err = ku.init()
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), fmt.Sprintf("http://127.0.0.1:%d", kubeletPort), ku.kubeletApiEndpoint)
	assert.Equal(suite.T(), "", ku.kubeletApiRequestHeaders.Get(authorizationHeaderKey))
	assert.True(suite.T(), ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), 0, len(ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.Certificates))

	require.EqualValues(suite.T(),
		map[string]string{
			"url": fmt.Sprintf("http://127.0.0.1:%d", kubeletPort),
			// token must be unset
		}, ku.GetRawConnectionInfo())
}

func (suite *KubeletTestSuite) TestKubeletInitHttp() {
	mockConfig := config.NewMock()

	// without token, without certs on HTTP
	k, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)

	s, kubeletPort, err := k.Start()
	require.Nil(suite.T(), err)
	defer s.Close()

	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

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
	assert.Equal(suite.T(), 0, len(ku.kubeletApiClient.Transport.(*http.Transport).TLSClientConfig.Certificates))

	require.EqualValues(suite.T(),
		map[string]string{
			"url": fmt.Sprintf("http://127.0.0.1:%d", kubeletPort),
		}, ku.GetRawConnectionInfo())
}

func TestKubeletTestSuite(t *testing.T) {
	config.SetupLogger(
		"trace",
		"",
		"",
		false,
		true,
		false,
	)
	suite.Run(t, new(KubeletTestSuite))
}
