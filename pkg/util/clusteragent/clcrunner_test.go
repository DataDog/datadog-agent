// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type dummyCLCRunner struct {
	rawResponses map[string]string
	requests     chan *http.Request
	sync.RWMutex
	token string
}

// resetGlobalCLCRunnerClient is a helper to remove the current CLCRunnerClient global
func resetGlobalCLCRunnerClient() {
	globalCLCRunnerClient = &CLCRunnerClient{}
	globalCLCRunnerClient.init()
}

func newDummyCLCRunner() (*dummyCLCRunner, error) {
	resetGlobalCLCRunnerClient()
	clcRunner := &dummyCLCRunner{
		rawResponses: map[string]string{
			"/api/v1/clcrunner/version": `{"Major":0, "Minor":0, "Patch":0, "Pre":"test", "Meta":"test", "Commit":"1337"}`,
			"/api/v1/clcrunner/stats":   `{"http_check:My Nginx Service:b0041608e66d20ba":{"AverageExecutionTime":241,"MetricSamples":3},"kube_apiserver_metrics:c5d2d20ccb4bb880":{"AverageExecutionTime":858,"MetricSamples":1562},"":{"AverageExecutionTime":100,"MetricSamples":10}}`,
			"/api/v1/clcrunner/workers": `{"Count":2,"Instances":{"worker_1":{"Utilization":0.1},"worker_2":{"Utilization":0.2}}}`,
		},
		token:    config.Datadog().GetString("cluster_agent.auth_token"),
		requests: make(chan *http.Request, 100),
	}
	return clcRunner, nil
}

func (d *dummyCLCRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("dummyCLCRunner received %s on %s", r.Method, r.URL.Path)
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

	// Handle raw responses if listed
	d.RLock()
	response, found := d.rawResponses[r.URL.Path]
	d.RUnlock()
	if found {
		w.Write([]byte(response))
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (d *dummyCLCRunner) parsePort(ts *httptest.Server) (*httptest.Server, int, error) {
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

func (d *dummyCLCRunner) StartTLS() (*httptest.Server, int, error) {
	ts := httptest.NewTLSServer(d)
	return d.parsePort(ts)
}

func (d *dummyCLCRunner) PopRequest() *http.Request {
	select {
	case r := <-d.requests:
		return r
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

type clcRunnerSuite struct {
	suite.Suite
	authTokenPath string
}

const (
	clcRunnerTokenValue = "01234567890123456789012345678901"
)

func (suite *clcRunnerSuite) SetupTest() {
	os.Remove(suite.authTokenPath)
	mockConfig.SetWithoutSource("cluster_agent.auth_token", clcRunnerTokenValue)
}

func (suite *clcRunnerSuite) TestGetCLCRunnerStats() {
	clcRunner, err := newDummyCLCRunner()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := clcRunner.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	c, err := GetCLCRunnerClient()
	c.(*CLCRunnerClient).clcRunnerPort = p

	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	expected := types.CLCRunnersStats{
		"http_check:My Nginx Service:b0041608e66d20ba": {
			AverageExecutionTime: 241,
			MetricSamples:        3,
		},
		"kube_apiserver_metrics:c5d2d20ccb4bb880": {
			AverageExecutionTime: 858,
			MetricSamples:        1562,
		},
	}

	suite.T().Run("", func(t *testing.T) {
		stats, err := c.GetRunnerStats("127.0.0.1")
		t.Logf("stats: %v", stats)

		require.Nil(t, err, fmt.Sprintf("%v", err))
		assert.Equal(t, expected, stats)
	})
}

func (suite *clcRunnerSuite) TestGetCLCRunnerVersion() {
	clcRunner, err := newDummyCLCRunner()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	ts, p, err := clcRunner.StartTLS()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	defer ts.Close()

	c, err := GetCLCRunnerClient()
	c.(*CLCRunnerClient).clcRunnerPort = p

	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))

	expected := version.Version{
		Major:  0,
		Minor:  0,
		Patch:  0,
		Pre:    "test",
		Meta:   "test",
		Commit: "1337",
	}

	suite.T().Run("", func(t *testing.T) {
		version, err := c.GetVersion("127.0.0.1")
		t.Logf("version: %v", version)

		require.Nil(t, err, fmt.Sprintf("%v", err))
		assert.Equal(t, expected, version)
	})
}

func (suite *clcRunnerSuite) TestGetRunnerWorkers() {
	clcRunner, err := newDummyCLCRunner()
	require.NoError(suite.T(), err)

	ts, p, err := clcRunner.StartTLS()
	require.NoError(suite.T(), err)
	defer ts.Close()

	c, err := GetCLCRunnerClient()
	require.NoError(suite.T(), err)

	c.(*CLCRunnerClient).clcRunnerPort = p

	expected := types.Workers{
		Count: 2,
		Instances: map[string]types.WorkerInfo{
			"worker_1": {
				Utilization: 0.1,
			},
			"worker_2": {
				Utilization: 0.2,
			},
		},
	}

	suite.T().Run("", func(t *testing.T) {
		workers, err := c.GetRunnerWorkers("127.0.0.1")
		require.NoError(suite.T(), err)
		assert.Equal(t, expected, workers)
	})
}

func TestCLCRunnerSuite(t *testing.T) {
	clcRunnerAuthTokenFilename := "cluster_agent.auth_token"

	fakeDir := t.TempDir()

	f, err := os.CreateTemp(fakeDir, "fake-datadog-yaml-")
	require.Nil(t, err, fmt.Errorf("%v", err))
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})

	s := &clcRunnerSuite{}
	config.Datadog().SetConfigFile(f.Name())
	s.authTokenPath = filepath.Join(fakeDir, clcRunnerAuthTokenFilename)
	_, err = os.Stat(s.authTokenPath)
	require.NotNil(t, err, fmt.Sprintf("%v", err))

	suite.Run(t, s)
}
