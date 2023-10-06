// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"fmt"
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
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dummyClusterAgent struct {
	nodeLabels      map[string]map[string]string
	nodeAnnotations map[string]map[string]string
	responses       map[string][]string
	responsesByNode apiv1.MetadataResponse
	rawResponses    map[string]string
	requests        chan *http.Request
	sync.RWMutex
	token       string
	redirectURL string
}

func newDummyClusterAgent() (*dummyClusterAgent, error) {
	resetGlobalClusterAgentClient()
	dca := &dummyClusterAgent{
		nodeLabels: map[string]map[string]string{
			"node/node1": {
				"label1": "value",
				"label2": "value2",
			},
			"node/node2": {
				"label3": "value",
				"label2": "value4",
			},
		},
		nodeAnnotations: map[string]map[string]string{
			"node/node1": {
				"annotation1": "value",
				"annotation2": "value2",
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
		responsesByNode: apiv1.MetadataResponse{
			Nodes: map[string]*apiv1.MetadataResponseBundle{
				"node1": {
					Services: apiv1.NamespacesPodsStringsSet{
						"foo": {
							"pod-00001": sets.New("kube_service:svc1"),
							"pod-00002": sets.New("kube_service:svc1", "kube_service:svc2"),
						},
						"bar": {
							"pod-00004": sets.New("kube_service:svc2"),
						},
					},
				},
				"node2": {
					Services: apiv1.NamespacesPodsStringsSet{
						"foo": {
							"pod-00003": sets.New("kube_service:svc1"),
						},
					},
				},
			},
		},
		rawResponses: map[string]string{
			"/version":           `{"Major":0, "Minor":0, "Patch":0, "Pre":"test", "Meta":"test", "Commit":"1337"}`,
			"/api/v1/cluster/id": `"94e43011-177b-11ea-a4fe-42010a8401d2"`,
		},
		token:    config.Datadog.GetString("cluster_agent.auth_token"),
		requests: make(chan *http.Request, 100),
	}
	return dca, nil
}

func newDummyClusterAgentWithCFMetadata() (*dummyClusterAgent, error) {
	resetGlobalClusterAgentClient()
	dca := &dummyClusterAgent{
		rawResponses: map[string]string{
			"/api/v1/tags/cf/apps/cell1": `{"instance1": ["container_name:app1_0"]}`,
			"/api/v1/tags/cf/apps/cell2": `{"instance2": ["container_name:app1_1"], "instance3": ["container_name:app2_0", "instance:0"]}`,
			"/api/v1/tags/cf/apps/cell3": `{}`,
			"/version":                   `{"Major":0, "Minor":0, "Patch":0, "Pre":"test", "Meta":"test", "Commit":"1337"}`,
		},
		requests: make(chan *http.Request, 100),
		token:    config.Datadog.GetString("cluster_agent.auth_token"),
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

	podIP := r.Header.Get(RealIPHeader)
	if podIP != clcRunnerIP {
		log.Errorf("wrong clc runner IP: %s", podIP)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Handle http redirect
	d.RLock()
	redirectURL := d.redirectURL
	d.RUnlock()
	if redirectURL != "" && strings.Contains(r.URL.Path, "/api/v1/clusterchecks/") {
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
	// Cloudfoundry metadata case: /api/v1/tags/cf/apps/{nodename}
	case 7:
		nodeName := s[6]
		d.RLock()
		defer d.RUnlock()
		if tags, found := d.rawResponses[nodeName]; found {
			w.Write([]byte(tags))
			return
		}
	case 6:
		nodeName := s[5]
		d.RLock()
		defer d.RUnlock()
		switch s[4] {
		case "pod":
			if nodeResp, found := d.responsesByNode.Nodes[nodeName]; found {
				resp := apiv1.MetadataResponse{
					Nodes: map[string]*apiv1.MetadataResponseBundle{
						nodeName: nodeResp,
					},
				}
				b, err := json.Marshal(resp)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Write(b)
				return
			}
		case "node":
			switch s[3] {
			case "tags":
				key := fmt.Sprintf("node/%s", nodeName)
				labels, found := d.nodeLabels[key]
				if found {
					b, err := json.Marshal(labels)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.Write(b)
					return
				}
			case "annotations":
				key := fmt.Sprintf("node/%s", nodeName)
				labels, found := d.nodeAnnotations[key]
				if found {
					b, err := json.Marshal(labels)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.Write(b)
					return
				}
			}
		default:
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

var mockConfig = config.Mock(nil)

const (
	clusterAgentServiceName = "DATADOG_CLUSTER_AGENT"
	clusterAgentServiceHost = clusterAgentServiceName + "_SERVICE_HOST"
	clusterAgentServicePort = clusterAgentServiceName + "_SERVICE_PORT"
	clusterAgentTokenValue  = "01234567890123456789012345678901"
	clcRunnerIP             = "10.92.1.39"
)

func (suite *clusterAgentSuite) SetupTest() {
	os.Remove(suite.authTokenPath)
	mockConfig.Set("cluster_agent.auth_token", clusterAgentTokenValue)
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")
	mockConfig.Set("clc_runner_host", clcRunnerIP)
	os.Unsetenv(clusterAgentServiceHost)
	os.Unsetenv(clusterAgentServicePort)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointEmpty() {
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")

	_, err := GetClusterAgentEndpoint()
	require.NotNil(suite.T(), err)
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenEmpty() {
	mockConfig.Set("cluster_agent.auth_token", "")

	_, err := security.CreateOrGetClusterAgentAuthToken()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenEmptyFile() {
	mockConfig.Set("cluster_agent.auth_token", "")
	err := os.WriteFile(suite.authTokenPath, []byte(""), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	_, err = security.GetClusterAgentAuthToken()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenFileInvalid() {
	mockConfig.Set("cluster_agent.auth_token", "")
	err := os.WriteFile(suite.authTokenPath, []byte("tooshort"), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	_, err = security.GetClusterAgentAuthToken()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthToken() {
	const tokenFileValue = "abcdefabcdefabcdefabcdefabcdefabcdefabcdef"
	mockConfig.Set("cluster_agent.auth_token", "")
	err := os.WriteFile(suite.authTokenPath, []byte(tokenFileValue), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	t, err := security.GetClusterAgentAuthToken()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), tokenFileValue, t)
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenConfigPriority() {
	const tokenFileValue = "abcdefabcdefabcdefabcdefabcdefabcdefabcdef"
	mockConfig.Set("cluster_agent.auth_token", clusterAgentTokenValue)
	err := os.WriteFile(suite.authTokenPath, []byte(tokenFileValue), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	// load config token value instead of filesystem
	t, err := security.GetClusterAgentAuthToken()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), clusterAgentTokenValue, t)
}

func (suite *clusterAgentSuite) TestGetClusterAgentAuthTokenTooShort() {
	const tokenValue = "tooshort"
	mockConfig.Set("cluster_agent.auth_token", "")
	err := os.WriteFile(suite.authTokenPath, []byte(tokenValue), os.ModePerm)
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	_, err = security.GetClusterAgentAuthToken()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromUrl() {
	mockConfig.Set("cluster_agent.url", "https://127.0.0.1:8080")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")
	_, err := GetClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	mockConfig.Set("cluster_agent.url", "https://127.0.0.1")
	_, err = GetClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	mockConfig.Set("cluster_agent.url", "127.0.0.1")
	endpoint, err := GetClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "https://127.0.0.1", endpoint)

	mockConfig.Set("cluster_agent.url", "127.0.0.1:1234")
	endpoint, err = GetClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "https://127.0.0.1:1234", endpoint)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromUrlInvalid() {
	mockConfig.Set("cluster_agent.url", "http://127.0.0.1:8080")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "")
	_, err := GetClusterAgentEndpoint()
	require.NotNil(suite.T(), err)

	mockConfig.Set("cluster_agent.url", "tcp://127.0.0.1:8080")
	_, err = GetClusterAgentEndpoint()
	require.NotNil(suite.T(), err)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromKubernetesSvc() {
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	suite.T().Setenv(clusterAgentServiceHost, "127.0.0.1")
	suite.T().Setenv(clusterAgentServicePort, "443")

	endpoint, err := GetClusterAgentEndpoint()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "https://127.0.0.1:443", endpoint)
}

func (suite *clusterAgentSuite) TestGetClusterAgentEndpointFromKubernetesSvcEmpty() {
	mockConfig.Set("cluster_agent.url", "")
	mockConfig.Set("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	suite.T().Setenv(clusterAgentServiceHost, "127.0.0.1")
	suite.T().Setenv(clusterAgentServicePort, "")

	_, err := GetClusterAgentEndpoint()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))

	suite.T().Setenv(clusterAgentServiceHost, "")
	suite.T().Setenv(clusterAgentServicePort, "443")
	_, err = GetClusterAgentEndpoint()
	require.NotNil(suite.T(), err, fmt.Sprintf("%v", err))
}

func (suite *clusterAgentSuite) TestGetKubernetesNodeLabels() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	testSuite := []struct {
		nodeName string
		expected map[string]string
		errors   error
	}{
		{
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
			errors:   errors.NewRemoteServiceError(fmt.Sprintf("https://127.0.0.1:%d/api/v1/tags/node/fake", p), "404 Not Found"),
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

func (suite *clusterAgentSuite) TestGetKubernetesNodeAnnotations() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	testSuite := []struct {
		nodeName string
		expected map[string]string
		errors   error
	}{
		{
			nodeName: "node1",
			errors:   nil,
			expected: map[string]string{
				"annotation1": "value",
				"annotation2": "value2",
			},
		},
		{
			nodeName: "fake",
			expected: nil,
			errors:   errors.NewRemoteServiceError(fmt.Sprintf("https://127.0.0.1:%d/api/v1/annotations/node/fake", p), "404 Not Found"),
		},
	}
	for _, testCase := range testSuite {
		suite.T().Run("", func(t *testing.T) {
			annotations, err := ca.GetNodeAnnotations(testCase.nodeName)
			t.Logf("Annotations: %s", annotations)
			require.Equal(t, err, testCase.errors)
			require.Equal(t, len(testCase.expected), len(annotations))
			for key, val := range testCase.expected {
				assert.Contains(t, annotations[key], val)
			}
		})
	}
}

func (suite *clusterAgentSuite) TestGetKubernetesMetadataNames() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

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

func (suite *clusterAgentSuite) TestGetCFAppsMetadataForNode() {
	dca, err := newDummyClusterAgentWithCFMetadata()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	testSuite := []struct {
		nodeName     string
		expectedTags map[string][]string
	}{
		{
			nodeName:     "cell1",
			expectedTags: map[string][]string{"instance1": {"container_name:app1_0"}},
		},
		{
			nodeName:     "cell2",
			expectedTags: map[string][]string{"instance2": {"container_name:app1_1"}, "instance3": {"container_name:app2_0", "instance:0"}},
		},
		{
			nodeName:     "cell3",
			expectedTags: map[string][]string{},
		},
	}
	for _, testCase := range testSuite {
		suite.T().Run("", func(t *testing.T) {
			tags, err := ca.GetCFAppsMetadataForNode(testCase.nodeName)
			t.Logf("tags: %s", tags)

			require.Nil(t, err, fmt.Sprintf("%v", err))
			require.Equal(t, len(testCase.expectedTags), len(tags))
			assert.EqualValues(t, testCase.expectedTags, tags)
		})
	}
}

func (suite *clusterAgentSuite) TestGetPodsMetadataForNode() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	testSuite := []struct {
		name              string
		nodeName          string
		expectedMetadatas apiv1.NamespacesPodsStringsSet
		expectedErr       error
	}{
		{
			name:     "basic case with 2 namespaces",
			nodeName: "node1",
			expectedMetadatas: apiv1.NamespacesPodsStringsSet{
				"foo": apiv1.MapStringSet{
					"pod-00001": sets.New("kube_service:svc1"),
					"pod-00002": sets.New("kube_service:svc1", "kube_service:svc2"),
				},
				"bar": {
					"pod-00004": sets.New("kube_service:svc2"),
				},
			},
		},
		{
			name:     "basic case",
			nodeName: "node2",
			expectedMetadatas: apiv1.NamespacesPodsStringsSet{
				"foo": apiv1.MapStringSet{
					"pod-00003": sets.New("kube_service:svc1"),
				},
			},
		},
		{
			name:        "error case: node not found",
			nodeName:    "node3",
			expectedErr: errors.NewRemoteServiceError(fmt.Sprintf("https://127.0.0.1:%d/api/v1/tags/pod/node3", p), "404 Not Found"),
		},
	}

	for _, testCase := range testSuite {
		suite.T().Run(testCase.name, func(t *testing.T) {
			metadatas, err := ca.GetPodsMetadataForNode(testCase.nodeName)
			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, testCase.expectedErr, err)
			} else {
				assert.NoError(t, err)
				t.Logf("metadatas: %s", metadatas)

				require.Nil(t, err, fmt.Sprintf("%v", err))
				require.Equal(t, len(testCase.expectedMetadatas), len(metadatas))
				for ns, expectedNSValues := range testCase.expectedMetadatas {
					for podName, expectedMetadatas := range expectedNSValues {
						for _, elt := range sets.List(expectedMetadatas) {
							assert.Contains(t, sets.List(metadatas[ns][podName]), elt)
						}
					}
				}
			}
		})
	}
}

func (suite *clusterAgentSuite) TestGetKubernetesClusterID() {
	dca, err := newDummyClusterAgent()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := dca.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	clusterID, err := ca.GetKubernetesClusterID()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), "94e43011-177b-11ea-a4fe-42010a8401d2", clusterID)
}

func TestClusterAgentSuite(t *testing.T) {
	clusterAgentAuthTokenFilename := "cluster_agent.auth_token"

	fakeDir := t.TempDir()

	f, err := os.CreateTemp(fakeDir, "fake-datadog-yaml-")
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
