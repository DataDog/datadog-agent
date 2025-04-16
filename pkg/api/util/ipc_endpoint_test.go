// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

type IPCEndpointTestSuite struct {
	suite.Suite
	conf pkgconfigmodel.Config
}

func TestIPCEndpointTestSuite(t *testing.T) {
	// cleaning auth_token and cert globals to be able initialize again the authToken and IPC cert
	token = ""
	dcaToken = ""
	clientTLSConfig = nil
	serverTLSConfig = nil
	initSource = uninitialized

	// creating test suite
	testSuite := new(IPCEndpointTestSuite)

	// simulating a normal startup of Agent with auth_token and cert generation
	testSuite.conf = configmock.New(t)

	// create a fake auth token
	dir := t.TempDir()
	authTokenPath := filepath.Join(dir, "auth_token")
	err := os.WriteFile(authTokenPath, []byte("0123456789abcdef0123456789abcdef"), 0640)
	require.NoError(t, err)
	testSuite.conf.Set("auth_token_file_path", authTokenPath, pkgconfigmodel.SourceAgentRuntime)

	// use the cert in the httptest server
	CreateAndSetAuthToken(testSuite.conf)

	suite.Run(t, testSuite)
}

func (suite *IPCEndpointTestSuite) setTestServerAndConfig(t *testing.T, ts *httptest.Server, isHTTPS bool) func() {
	if isHTTPS {
		ts.TLS = GetTLSServerConfig()
		ts.StartTLS()
	} else {
		ts.Start()
	}

	// use the httptest server as the CMD_API
	addr, err := url.Parse(ts.URL)
	require.NoError(t, err)
	localHost, localPort, _ := net.SplitHostPort(addr.Host)
	suite.conf.Set("cmd_host", localHost, pkgconfigmodel.SourceAgentRuntime)
	suite.conf.Set("cmd_port", localPort, pkgconfigmodel.SourceAgentRuntime)

	return func() {
		ts.Close()
		suite.conf.UnsetForSource("cmd_host", pkgconfigmodel.SourceAgentRuntime)
		suite.conf.UnsetForSource("cmd_port", pkgconfigmodel.SourceAgentRuntime)
	}
}

func (suite *IPCEndpointTestSuite) TestNewIPCEndpoint() {
	t := suite.T()

	// set minimal configuration that IPCEndpoint needs
	suite.conf.Set("cmd_host", "localhost", pkgconfigmodel.SourceAgentRuntime)
	suite.conf.Set("cmd_port", "6789", pkgconfigmodel.SourceAgentRuntime)

	// test the endpoint construction
	end, err := NewIPCEndpoint(suite.conf, "test/api")
	require.NoError(t, err)
	assert.Equal(t, end.target.String(), "https://localhost:6789/test/api")
}

func (suite *IPCEndpointTestSuite) TestNewIPCEndpointWithCloseConnection() {
	t := suite.T()

	// test constructing with the CloseConnection option
	end, err := NewIPCEndpoint(suite.conf, "test/api", WithCloseConnection(true))
	require.NoError(t, err)
	assert.True(t, end.closeConn)
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointDoGet() {
	t := suite.T()
	gotURL := ""
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	clean := suite.setTestServerAndConfig(t, ts, true)
	defer clean()

	end, err := NewIPCEndpoint(suite.conf, "test/api")
	require.NoError(t, err)

	// test that DoGet will hit the endpoint url
	res, err := end.DoGet()
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api")
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointGetWithHTTPClientAndNonTLS() {
	t := suite.T()
	// non-http server
	gotURL := ""
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	// create non-TLS client and use the "http" protocol
	clean := suite.setTestServerAndConfig(t, ts, false)
	defer clean()

	client := http.Client{}
	end, err := NewIPCEndpoint(suite.conf, "test/api", WithHTTPClient(&client), WithURLScheme("http"))
	require.NoError(t, err)

	// test that DoGet will hit the endpoint url
	res, err := end.DoGet()
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api")
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointGetWithValues() {
	t := suite.T()
	gotURL := ""
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	clean := suite.setTestServerAndConfig(t, ts, true)
	defer clean()

	// set url values for GET request
	v := url.Values{}
	v.Set("verbose", "true")

	// test construction with option for url.Values
	end, err := NewIPCEndpoint(suite.conf, "test/api")
	require.NoError(t, err)

	// test that DoGet will use query parameters from the url.Values
	res, err := end.DoGet(WithValues(v))
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api?verbose=true")
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointGetWithHostAndPort() {
	t := suite.T()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	clean := suite.setTestServerAndConfig(t, ts, true)
	defer clean()

	// modify the config so that it uses a different setting for the cmd_host
	suite.conf.Set("process_config.cmd_host", "127.0.0.1", pkgconfigmodel.SourceAgentRuntime)

	// test construction with alternate values for the host and port
	end, err := NewIPCEndpoint(suite.conf, "test/api", WithHostAndPort(suite.conf.GetString("process_config.cmd_host"), suite.conf.GetInt("cmd_port")))
	require.NoError(t, err)

	// test that host provided by WithHostAndPort is used for the endpoint
	res, err := end.DoGet()
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, end.target.Host, fmt.Sprintf("127.0.0.1:%d", suite.conf.GetInt("cmd_port")))
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointDeprecatedIPCAddress() {
	t := suite.T()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	clean := suite.setTestServerAndConfig(t, ts, true)
	defer clean()
	// Use the deprecated (but still supported) option "ipc_address"
	suite.conf.UnsetForSource("cmd_host", pkgconfigmodel.SourceAgentRuntime)
	suite.conf.Set("ipc_address", "127.0.0.1", pkgconfigmodel.SourceAgentRuntime)
	defer suite.conf.UnsetForSource("ipc_address", pkgconfigmodel.SourceAgentRuntime)

	// test construction, uses ipc_address instead of cmd_host
	end, err := NewIPCEndpoint(suite.conf, "test/api")
	require.NoError(t, err)

	// test that host provided by "ipc_address" is used for the endpoint
	res, err := end.DoGet()
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, end.target.Host, fmt.Sprintf("127.0.0.1:%d", suite.conf.GetInt("cmd_port")))
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointErrorText() {
	t := suite.T()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))
	clean := suite.setTestServerAndConfig(t, ts, true)
	defer clean()

	end, err := NewIPCEndpoint(suite.conf, "test/api")
	require.NoError(t, err)

	// test that error is returned by the endpoint
	_, err = end.DoGet()
	require.Error(t, err)
}

func (suite *IPCEndpointTestSuite) TestIPCEndpointErrorMap() {
	t := suite.T()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		data, _ := json.Marshal(map[string]string{
			"error": "something went wrong",
		})
		w.Write(data)
	}))
	clean := suite.setTestServerAndConfig(t, ts, true)
	defer clean()

	end, err := NewIPCEndpoint(suite.conf, "test/api")
	require.NoError(t, err)

	// test that error gets unwrapped from the errmap
	_, err = end.DoGet()
	require.Error(t, err)
	assert.Equal(t, err.Error(), "something went wrong")
}
