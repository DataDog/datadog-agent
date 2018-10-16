// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dummyClusterAgent struct {
	node         map[string]map[string]string
	responses    map[string][]string
	rawResponses map[string]string
	requests     chan *http.Request
	sync.RWMutex
	token       string
	redirectURL string
}

func newDummyClusterAgent() (*dummyClusterAgent, error) {
	resetGlobalClusterAgentClient()
	dca := &dummyClusterAgent{
		node: map[string]map[string]string{
			"node/node1": {
				"label1": "value",
				"label2": "value2",
			},
			"node/node2": {
				"label3": "value",
				"label2": "value4",
			},
		},
		responses: map[string][]string{
			"pod/node1/foo/pod-00001": {"kube_service:svc1"},
			"pod/node1/foo/pod-00002": {"kube_service:svc1", "kube_service:svc2"},
			"pod/node1/foo/pod-00003": {"kube_service:svc1"},
			"pod/node2/bar/pod-00004": {"kube_service:svc2"},
			"pod/node2/bar/pod-00005": {"kube_service:svc3"},
			"pod/node2/bar/pod-00006": {},
		},
		rawResponses: map[string]string{},
		token:        config.Datadog.GetString("cluster_agent.auth_token"),
		requests:     make(chan *http.Request, 6),
	}
	return dca, nil
}

func (d *dummyClusterAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("dummyDCA received %s on %s", r.Method, r.URL.Path)
	d.requests <- r

	token := r.Header.Get("Authorization")
	if token == "" {
		log.Errorf("no token provided")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if token != fmt.Sprintf("Bearer %s", d.token) {
		log.Errorf("wrong token %s", token)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Handle http redirect
	d.RLock()
	redirectURL := d.redirectURL
	d.RUnlock()
	if redirectURL != "" {
		url, _ := url.Parse(redirectURL)
		url.Path = r.URL.Path
		http.Redirect(w, r, url.String(), http.StatusFound)
	}

	// Handle raw responses if listed
	d.RLock()
	response, found := d.rawResponses[r.URL.Path]
	d.RUnlock()
	if found {
		w.Write([]byte(response))
		return
	}

	// path should be like: /api/v1/tags/pod/{nodeName}/{ns}/{pod-[0-9a-z]+}
	s := strings.Split(r.URL.Path, "/")
	switch len(s) {
	case 8:
		nodeName, ns, podName := s[5], s[6], s[7]
		key := fmt.Sprintf("pod/%s/%s/%s", nodeName, ns, podName)
		d.RLock()
		defer d.RUnlock()
		svcs, found := d.responses[key]
		if found {
			b, err := json.Marshal(svcs)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write(b)
			return
		}
	case 6:
		nodeName := s[5]
		key := fmt.Sprintf("node/%s", nodeName)
		d.RLock()
		defer d.RUnlock()
		labels, found := d.node[key]
		if found {
			b, err := json.Marshal(labels)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write(b)
			return
		}
	default:
		w.WriteHeader(http.StatusInternalServerError)
		log.Errorf("unexpected len for the url != %d", len(s))
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (d *dummyClusterAgent) parsePort(ts *httptest.Server) (*httptest.Server, int, error) {
	u, err := url.Parse(ts.URL)
	if err != nil {
		return nil, 0, err
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, 0, err
	}
	return ts, p, nil
}

func (d *dummyClusterAgent) StartTLS() (*httptest.Server, int, error) {
	ts := httptest.NewTLSServer(d)
	return d.parsePort(ts)
}

func (d *dummyClusterAgent) PopRequest() *http.Request {
	select {
	case r := <-d.requests:
		return r
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

type clusterAgentSuite struct {
	suite.Suite
	authTokenPath string
}

var mockConfig = config.NewMock()

const (
	clusterAgentServiceName = "DATADOG_CLUSTER_AGENT"
	clusterAgentServiceHost = clusterAgentServiceName + "_SERVICE_HOST"
	clusterAgentServicePort = clusterAgentServiceName + "_SERVICE_PORT"
	clusterAgentTokenValue  = "01234567890123456789012345678901"
)

func (suite *clusterAgentSuite) SetupTest() {
	os.Remove(suite.authTokenPath)
	mockConfig.Set("cluster_agent.auth_token", clusterAgentTokenValue)
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")
	os.Unsetenv(clusterAgentServiceHost)
	os.Unsetenv(clusterAgentServicePort)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointEmpty() {
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")

	_, err := getClusterAgentEndpoint()
	require.NotNil(suite.T(), err)
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenEmpty() {
	mockConfig.Set("cluster_agent.auth_token", "")

	_, err := security.GetClusterAgentAuthToken()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenEmptyFile() {
	mockConfig.Set("cluster_agent.auth_token", "")
	err := ioutil.WriteFile(suite.authTokenPath, []byte(""), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	_, err = security.GetClusterAgentAuthToken()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenFileInvalid() {
	mockConfig.Set("cluster_agent.auth_token", "")
	err := ioutil.WriteFile(suite.authTokenPath, []byte("tooshort"), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	_, err = security.GetClusterAgentAuthToken()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthToken() {
	const tokenFileValue = "abcdefabcdefabcdefabcdefabcdefabcdefabcdef"
	mockConfig.Set("cluster_agent.auth_token", "")
	err := ioutil.WriteFile(suite.authTokenPath, []byte(tokenFileValue), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	t, err := security.GetClusterAgentAuthToken()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), tokenFileValue, t)
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenConfigPriority() {
	const tokenFileValue = "abcdefabcdefabcdefabcdefabcdefabcdefabcdef"
	mockConfig.Set("cluster_agent.auth_token", clusterAgentTokenValue)
	err := ioutil.WriteFile(suite.authTokenPath, []byte(tokenFileValue), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	// load config token value instead of filesystem
	t, err := security.GetClusterAgentAuthToken()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), clusterAgentTokenValue, t)
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenTooShort() {
	const tokenValue = "tooshort"
	mockConfig.Set("cluster_agent.auth_token", "")
	err := ioutil.WriteFile(suite.authTokenPath, []byte(tokenValue), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	_, err = security.GetClusterAgentAuthToken()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromUrl() {
	mockConfig.Set("cluster_agent.url", "https://127.0.0.1:8080")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")
	_, err := getClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	mockConfig.Set("cluster_agent.url", "https://127.0.0.1")
	_, err = getClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	mockConfig.Set("cluster_agent.url", "127.0.0.1")
	endpoint, err := getClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "https://127.0.0.1", endpoint)

	mockConfig.Set("cluster_agent.url", "127.0.0.1:1234")
	endpoint, err = getClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "https://127.0.0.1:1234", endpoint)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromUrlInvalid() {
	mockConfig.Set("cluster_agent.url", "http://127.0.0.1:8080")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")
	_, err := getClusterAgentEndpoint()
	require.NotNil(suite.T(), err)

	mockConfig.Set("cluster_agent.url", "tcp://127.0.0.1:8080")
	_, err = getClusterAgentEndpoint()
	require.NotNil(suite.T(), err)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromKubernetesSvc() {
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	os.Setenv(clusterAgentServiceHost, "127.0.0.1")
	os.Setenv(clusterAgentServicePort, "443")

	endpoint, err := getClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "https://127.0.0.1:443", endpoint)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromKubernetesSvcEmpty() {
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	os.Setenv(clusterAgentServiceHost, "127.0.0.1")
	os.Setenv(clusterAgentServicePort, "")

	_, err := getClusterAgentEndpoint()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))

	os.Setenv(clusterAgentServiceHost, "")
	os.Setenv(clusterAgentServicePort, "443")
	_, err = getClusterAgentEndpoint()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetKubernetesNodeLabels() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	defer ts.Close()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	testSuite := []struct {
		nodeName string
		expected map[string]string
		errors   error
	}{{
		nodeName: "node1",
		errors:   nil,
		expected: map[string]string{
			"label1": "value",
			"label2": "value2",
		},
	},
		{
			nodeName: "node2",
			expected: map[string]string{
				"label3": "value",
				"label2": "value4",
			},
			errors: nil,
		},
		{
			nodeName: "fake",
			expected: nil,
			errors:   fmt.Errorf("unexpected status code from cluster agent: 404"),
		},
	}
	for _, testCase := range testSuite {
		suite.T().Run("", func(t *testing.T) {
			labels, err := ca.GetNodeLabels(testCase.nodeName)
			t.Logf("Labels: %s", labels)
			require.Equal(t, err, testCase.errors)
			require.Equal(t, len(testCase.expected), len(labels))
			for key, val := range testCase.expected {
				assert.Contains(t, labels[key], val)
			}
		})
	}
}

func (suite *clusterAgentSuite) TestGetKubernetesMetadataNames() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	defer ts.Close()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	testSuite := []struct {
		nodeName    string
		podName     string
		namespace   string
		expectedSvc []string
	}{
		{
			nodeName:    "node1",
			podName:     "pod-00001",
			namespace:   "foo",
			expectedSvc: []string{"kube_service:svc1"},
		},
		{
			nodeName:    "node1",
			podName:     "pod-00002",
			namespace:   "foo",
			expectedSvc: []string{"kube_service:svc1", "kube_service:svc2"},
		},
		{
			nodeName:    "node1",
			podName:     "pod-00003",
			namespace:   "foo",
			expectedSvc: []string{"kube_service:svc1"},
		},
		{
			nodeName:    "node2",
			podName:     "pod-00004",
			namespace:   "bar",
			expectedSvc: []string{"kube_service:svc2"},
		},
		{
			nodeName:    "node2",
			podName:     "pod-00005",
			namespace:   "bar",
			expectedSvc: []string{"kube_service:svc3"},
		},
		{
			nodeName:    "node2",
			podName:     "pod-00006",
			namespace:   "bar",
			expectedSvc: []string{},
		},
	}
	for _, testCase := range testSuite {
		suite.T().Run("", func(t *testing.T) {
			svc, err := ca.GetKubernetesMetadataNames(testCase.nodeName, testCase.namespace, testCase.podName)
			t.Logf("svc: %s", svc)

			require.Nil(t, err, fmt.Sprintf("%v", err))
			require.Equal(t, len(testCase.expectedSvc), len(svc))
			for _, elt := range testCase.expectedSvc {
				assert.Contains(t, svc, elt)
			}
		})
	}
}

func TestClusterAgentSuite(t *testing.T) {
	clusterAgentAuthTokenFilename := "cluster_agent.auth_token"

	fakeDir, err := ioutil.TempDir("", "fake-datadog-etc")
	require.Nil(t, err, fmt.Sprintf("%v", err))
	defer os.RemoveAll(fakeDir)

	f, err := ioutil.TempFile(fakeDir, "fake-datadog-yaml-")
	require.Nil(t, err, fmt.Errorf("%v", err))
	defer os.Remove(f.Name())

	s := &clusterAgentSuite{}
	config.Datadog.SetConfigFile(f.Name())
	s.authTokenPath = filepath.Join(fakeDir, clusterAgentAuthTokenFilename)
	_, err = os.Stat(s.authTokenPath)
	require.NotNil(t, err, fmt.Sprintf("%v", err))
	defer os.Remove(s.authTokenPath)

	suite.Run(t, s)
}
