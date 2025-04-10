// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func getMockServerAndConfig(t testing.TB, handler http.Handler) (ipc.HTTPClient, *httptest.Server) {

	conf := config.NewMock(t)
	util.SetAuthTokenInMemory(t)

	ts := httptest.NewUnstartedServer(handler)
	ts.TLS = util.GetTLSServerConfig()
	ts.StartTLS()
	t.Cleanup(ts.Close)

	// set the cmd_host and cmd_port in the config
	addr, err := url.Parse(ts.URL)
	require.NoError(t, err)
	localHost, localPort, _ := net.SplitHostPort(addr.Host)
	conf.Set("cmd_host", localHost, pkgconfigmodel.SourceAgentRuntime)
	conf.Set("cmd_port", localPort, pkgconfigmodel.SourceAgentRuntime)
	t.Cleanup(func() {
		conf.UnsetForSource("cmd_host", pkgconfigmodel.SourceAgentRuntime)
		conf.UnsetForSource("cmd_port", pkgconfigmodel.SourceAgentRuntime)
	})

	return NewClient(util.GetAuthToken(), util.GetTLSClientConfig(), conf), ts
}

func TestWithInsecureTransport(t *testing.T) {

	// Spinning up server with IPC cert
	knownHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("secure"))
	}
	client, knownServer := getMockServerAndConfig(t, http.HandlerFunc(knownHandler))

	// Spinning up server with self-signed cert
	unknownHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("insecure"))
	}
	unknownServer := httptest.NewTLSServer(http.HandlerFunc(unknownHandler))
	t.Cleanup(unknownServer.Close)

	t.Run("secure client with known server: must succeed", func(t *testing.T) {
		// Intenting a secure request
		body, err := client.Get(knownServer.URL)
		require.NoError(t, err)
		require.Equal(t, "secure", string(body))
	})

	t.Run("secure client with unknown server: must fail", func(t *testing.T) {
		// Intenting a secure request
		_, err := client.Get(unknownServer.URL)
		require.Error(t, err)
	})
}

func TestDoGet(t *testing.T) {
	t.Run("simple request", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("test"))
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler))
		data, err := client.Get(ts.URL)
		require.NoError(t, err)
		require.Equal(t, "test", string(data))
	})

	t.Run("error response", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler))
		_, err := client.Get(ts.URL)
		require.Error(t, err)
	})

	t.Run("url error", func(t *testing.T) {
		client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		_, err := client.Get(" http://localhost")
		require.Error(t, err)
	})

	t.Run("request error", func(t *testing.T) {
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		ts.Close()

		_, err := client.Get(ts.URL)
		require.Error(t, err)
	})

	t.Run("check auth token", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+util.GetAuthToken(), r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler))

		data, err := client.Get(ts.URL)
		require.NoError(t, err)
		require.Empty(t, data)
	})

	t.Run("context cancel", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.Get(ts.URL, WithContext(ctx))
		require.Error(t, err)
	})
}

func TestNewIPCEndpoint(t *testing.T) {
	// test the endpoint construction
	client, ts := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	inner, ok := end.(*IPCEndpoint)
	require.True(t, ok)

	assert.Equal(t, inner.target.String(), ts.URL+"/test/api")
}

func TestIPCEndpointDoGet(t *testing.T) {
	gotURL := ""
	client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))

	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	// test that DoGet will hit the endpoint url
	res, err := end.DoGet()
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api")
}

func TestIPCEndpointGetWithValues(t *testing.T) {
	gotURL := ""
	client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))

	// set url values for GET request
	v := url.Values{}
	v.Set("verbose", "true")

	// test construction with option for url.Values
	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	// test that DoGet will use query parameters from the url.Values
	res, err := end.DoGet(WithValues(v))
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, gotURL, "/test/api?verbose=true")
}

func TestIPCEndpointDeprecatedIPCAddress(t *testing.T) {
	client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))

	conf := config.NewMock(t)
	// Use the deprecated (but still supported) option "ipc_address"
	conf.UnsetForSource("cmd_host", pkgconfigmodel.SourceAgentRuntime)
	conf.Set("ipc_address", "127.0.0.1", pkgconfigmodel.SourceAgentRuntime)
	defer conf.UnsetForSource("ipc_address", pkgconfigmodel.SourceAgentRuntime)

	// test construction, uses ipc_address instead of cmd_host
	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	inner, ok := end.(*IPCEndpoint)
	require.True(t, ok)

	// test that host provided by "ipc_address" is used for the endpoint
	res, err := end.DoGet()
	require.NoError(t, err)
	assert.Equal(t, res, []byte("ok"))
	assert.Equal(t, inner.target.Host, fmt.Sprintf("127.0.0.1:%d", conf.GetInt("cmd_port")))
}

func TestIPCEndpointErrorText(t *testing.T) {
	client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))

	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	// test that error is returned by the endpoint
	_, err = end.DoGet()
	require.Error(t, err)
}

func TestIPCEndpointErrorMap(t *testing.T) {
	client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		data, _ := json.Marshal(map[string]string{
			"error": "something went wrong",
		})
		w.Write(data)
	}))

	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	// test that error gets unwrapped from the errmap
	_, err = end.DoGet()
	require.Error(t, err)
	assert.Equal(t, err.Error(), "something went wrong")
}
