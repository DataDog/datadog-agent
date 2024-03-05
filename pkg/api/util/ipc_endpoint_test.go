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
	"strings"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
)

func createConfig(t *testing.T, ts *httptest.Server) pkgconfigmodel.Config {
	conf := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	// create a fake auth token
	authTokenFile, err := os.CreateTemp("", "")
	assert.NoError(t, err)
	authTokenPath := authTokenFile.Name()
	os.WriteFile(authTokenPath, []byte("0123456789abcdef0123456789abcdef"), 0640)

	addr, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	localHost, localPort, _ := net.SplitHostPort(addr.Host)

	// set minimal configuration that IPCEndpoint needs
	conf.Set("auth_token_file_path", authTokenPath, pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_host", localHost, pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_port", localPort, pkgconfigmodel.SourceAgentRuntime)

	return conf
}

func TestNewIPCEndpoint(t *testing.T) {
	conf := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	// create a fake auth token
	authTokenFile, err := os.CreateTemp("", "")
	assert.NoError(t, err)
	authTokenPath := authTokenFile.Name()
	os.WriteFile(authTokenPath, []byte("0123456789abcdef0123456789abcdef"), 0640)

	// set minimal configuration that IPCEndpoint needs
	conf.Set("auth_token_file_path", authTokenPath, pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_host", "localhost", pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_port", "6789", pkgconfigmodel.SourceAgentRuntime)

	// test the endpoint construction
	end, err := NewIPCEndpoint(conf, "test/api")
	assert.NoError(t, err)
	assert.Equal(t, end.target.String(), "https://localhost:6789/test/api")
}

func TestNewIPCEndpointWithCloseConnection(t *testing.T) {
	conf := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	// create a fake auth token
	authTokenFile, err := os.CreateTemp("", "")
	assert.NoError(t, err)
	authTokenPath := authTokenFile.Name()
	os.WriteFile(authTokenPath, []byte("0123456789abcdef0123456789abcdef"), 0640)

	// set minimal configuration that IPCEndpoint needs
	conf.Set("auth_token_file_path", authTokenPath, pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_host", "localhost", pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_port", "6789", pkgconfigmodel.SourceAgentRuntime)

	// test constructing with the CloseConnection option
	end, err := NewIPCEndpoint(conf, "test/api", WithCloseConnection(true))
	assert.NoError(t, err)
	assert.True(t, end.closeConn)
}

func TestIPCEndpointDoGet(t *testing.T) {
	gotURL := ""
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	conf := createConfig(t, ts)
	end, err := NewIPCEndpoint(conf, "test/api")
	assert.NoError(t, err)

	// test that DoGet will hit the endpoint url
	res, err := end.DoGet()
	assert.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api")
}

func TestIPCEndpointGetWithHTTPClientAndNonTLS(t *testing.T) {
	// non-http server
	gotURL := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// create non-TLS client and use the "http" protocol
	client := http.Client{}
	conf := createConfig(t, ts)
	end, err := NewIPCEndpoint(conf, "test/api", WithHTTPClient(&client), WithURLScheme("http"))
	assert.NoError(t, err)

	// test that DoGet will hit the endpoint url
	res, err := end.DoGet()
	assert.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api")
}

func TestIPCEndpointGetWithValues(t *testing.T) {
	gotURL := ""
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	conf := createConfig(t, ts)
	// set url values for GET request
	v := url.Values{}
	v.Set("verbose", "true")

	// test construction with option for url.Values
	end, err := NewIPCEndpoint(conf, "test/api")
	assert.NoError(t, err)

	// test that DoGet will use query parameters from the url.Values
	res, err := end.DoGet(WithValues(v))
	assert.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api?verbose=true")
}

func TestIPCEndpointGetWithHostAndPort(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	conf := createConfig(t, ts)
	// modify the config so that it uses a different setting for the cmd_host
	conf.Set("process_config.cmd_host", "127.0.0.1", pkgconfigmodel.SourceAgentRuntime)

	// test construction with alternate values for the host and port
	end, err := NewIPCEndpoint(conf, "test/api", WithHostAndPort(conf.GetString("process_config.cmd_host"), conf.GetInt("cmd_port")))
	assert.NoError(t, err)

	// test that host provided by WithHostAndPort is used for the endpoint
	res, err := end.DoGet()
	assert.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, end.target.Host, fmt.Sprintf("127.0.0.1:%d", conf.GetInt("cmd_port")))
}

func TestIPCEndpointDeprecatedIPCAddress(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	conf := createConfig(t, ts)
	// Use the deprecated (but still supported) option "ipc_address"
	conf.UnsetForSource("cmd_host", pkgconfigmodel.SourceAgentRuntime)
	conf.Set("ipc_address", "127.0.0.1", pkgconfigmodel.SourceAgentRuntime)

	// test construction, uses ipc_address instead of cmd_host
	end, err := NewIPCEndpoint(conf, "test/api")
	assert.NoError(t, err)

	// test that host provided by "ipc_address" is used for the endpoint
	res, err := end.DoGet()
	assert.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, end.target.Host, fmt.Sprintf("127.0.0.1:%d", conf.GetInt("cmd_port")))
}

func TestIPCEndpointErrorText(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))
	defer ts.Close()

	conf := createConfig(t, ts)
	end, err := NewIPCEndpoint(conf, "test/api")
	assert.NoError(t, err)

	// test that error is returned by the endpoint
	_, err = end.DoGet()
	assert.Error(t, err)
}

func TestIPCEndpointErrorMap(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		data, _ := json.Marshal(map[string]string{
			"error": "something went wrong",
		})
		w.Write(data)
	}))
	defer ts.Close()

	conf := createConfig(t, ts)
	end, err := NewIPCEndpoint(conf, "test/api")
	assert.NoError(t, err)

	// test that error gets unwrapped from the errmap
	_, err = end.DoGet()
	assert.Error(t, err)
	assert.Equal(t, err.Error(), "something went wrong")
}
