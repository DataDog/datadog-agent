// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package httphelpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const testToken = "test-token"

// The following certificate and key are used for testing purposes only.
// They have been generated using the following command:
//
//	openssl req -x509 -newkey ec:<(openssl ecparam -name prime256v1) -keyout key.pem -out cert.pem -days 3650 \
//	  -subj "/O=Datadog, Inc." \
//	  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
//	  -addext "keyUsage=digitalSignature,keyEncipherment" \
//	  -addext "extendedKeyUsage=serverAuth,clientAuth" \
//	  -addext "basicConstraints=CA:TRUE" \
//	  -nodes
var (
	unknownCAcert = []byte(`-----BEGIN CERTIFICATE-----
MIIByzCCAXKgAwIBAgIUNZPpHI4XP/vz/1NCwAyJ/VxaEk8wCgYIKoZIzj0EAwIw
GDEWMBQGA1UECgwNRGF0YWRvZywgSW5jLjAeFw0yNTA2MTYxMTMzNTFaFw0zNTA2
MTQxMTMzNTFaMBgxFjAUBgNVBAoMDURhdGFkb2csIEluYy4wWTATBgcqhkjOPQIB
BggqhkjOPQMBBwNCAAS2/cnezdb31wqnEzlsqCIYywWiDOoY47in1Kooh8qpn8p0
PxCBNCmfhe6U8KomhCKlOhVAarygsvlPZKgYXwuko4GZMIGWMB0GA1UdDgQWBBQ5
w7Z77E/8m68si1ncIkrT26MgnzAfBgNVHSMEGDAWgBQ5w7Z77E/8m68si1ncIkrT
26MgnzAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8AAAEwCwYDVR0PBAQDAgWgMB0G
A1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAMBgNVHRMEBTADAQH/MAoGCCqG
SM49BAMCA0cAMEQCIE5RYWT7lJ4xcezLkz23FP+vfQnK/iVGZJcJf9+pi2XOAiBp
rLpPpGVXs5I4phPz4oD9XIJfTo5tnRGsJ4+cM3YA+A==
-----END CERTIFICATE-----
`)
	unknownCAkey = []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgNgoNHbT7PkG0FR6T
K8v8KAngdgbh8ihjagt1zyqQbSOhRANCAAS2/cnezdb31wqnEzlsqCIYywWiDOoY
47in1Kooh8qpn8p0PxCBNCmfhe6U8KomhCKlOhVAarygsvlPZKgYXwuk
-----END PRIVATE KEY-----
`)
)

func getMockServerAndConfig(t testing.TB, handler http.Handler, token string) (ipc.HTTPClient, *httptest.Server) {
	conf := config.NewMock(t)

	ts := httptest.NewTLSServer(handler)
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

	// Extracting the TLS configuration from the test server client transport
	clientTransport, ok := ts.Client().Transport.(*http.Transport)
	require.True(t, ok, "Expected the transport to be of type *http.Transport")
	clientTLSConfig := clientTransport.TLSClientConfig

	return NewClient(token, clientTLSConfig, conf), ts
}

func TestWithInsecureTransport(t *testing.T) {

	// Spinning up server with IPC cert
	knownHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("secure"))
	}
	client, knownServer := getMockServerAndConfig(t, http.HandlerFunc(knownHandler), testToken)

	// Spinning up server with self-signed cert
	unknownHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("insecure"))
	}
	unknownServer := httptest.NewUnstartedServer(http.HandlerFunc(unknownHandler))
	// Setting up the server to use the self-signed cert
	_, tlsServerConfig, err := cert.GetTLSConfigFromCert(unknownCAcert, unknownCAkey)
	require.NoError(t, err, "Failed to get TLS config from self-signed cert")
	unknownServer.TLS = tlsServerConfig
	unknownServer.StartTLS()
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
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)
		data, err := client.Get(ts.URL)
		require.NoError(t, err)
		require.Equal(t, "test", string(data))
	})

	t.Run("error response", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)
		_, err := client.Get(ts.URL)
		require.Error(t, err)
	})

	t.Run("url error", func(t *testing.T) {
		client, _ := getMockServerAndConfig(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}), testToken)
		_, err := client.Get(" http://localhost")
		require.Error(t, err)
	})

	t.Run("request error", func(t *testing.T) {
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}), testToken)
		ts.Close()

		_, err := client.Get(ts.URL)
		require.Error(t, err)
	})

	t.Run("check auth token", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testToken, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)

		data, err := client.Get(ts.URL)
		require.NoError(t, err)
		require.Empty(t, data)
	})

	t.Run("context cancel", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.Get(ts.URL, WithContext(ctx))
		require.Error(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
				t.Error("server handler did not unblock within 3 seconds")
			}
			w.WriteHeader(http.StatusOK)
		}
		client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)

		_, err := client.Get(ts.URL, WithTimeout(10*time.Millisecond))
		require.Error(t, err)

		var netErr net.Error
		require.True(t, errors.As(err, &netErr))
		assert.True(t, netErr.Timeout())
	})
}

func TestTLSHandshakeTimeoutClosesBlackholeConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	accepted := make(chan net.Conn, 1)
	peerClosed := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			peerClosed <- err
			return
		}
		defer conn.Close()
		accepted <- conn

		// Consume the ClientHello, then deliberately never complete the TLS handshake.
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			peerClosed <- err
			return
		}
		if _, err := io.ReadFull(conn, make([]byte, int(header[3])<<8|int(header[4]))); err != nil {
			peerClosed <- err
			return
		}

		_, err = conn.Read(make([]byte, 1))
		peerClosed <- err
	}()

	var peer net.Conn
	t.Cleanup(func() {
		_ = listener.Close()
		if peer != nil {
			_ = peer.Close()
		}
	})

	conf := config.NewMock(t)
	conf.Set("server_timeout", 1, pkgconfigmodel.SourceAgentRuntime)
	conf.Set("tls_handshake_timeout", 100*time.Millisecond, pkgconfigmodel.SourceAgentRuntime)
	timeout := ipcTLSHandshakeTimeout(conf, conf.GetDuration("server_timeout")*time.Second)
	client := NewClient(testToken, nil, conf)
	requestDone := make(chan error, 1)
	go func() {
		_, err := client.Get("https://" + listener.Addr().String())
		requestDone <- err
	}()

	select {
	case peer = <-accepted:
	case err := <-peerClosed:
		require.NoError(t, err, "listener did not receive a TLS ClientHello")
	case <-time.After(time.Second):
		require.Fail(t, "client did not connect to the listener")
	}

	select {
	case err := <-requestDone:
		require.Error(t, err)
	case <-time.After(timeout + time.Second):
		require.Fail(t, "TLS handshake did not time out")
	}

	// The transport timeout must close the socket, not merely return from Client.Do.
	select {
	case err := <-peerClosed:
		require.Error(t, err)
	case <-time.After(time.Second):
		require.Fail(t, "TLS handshake socket remained open")
	}

	require.Eventually(t, func() bool {
		stacks := make([]byte, 1<<20)
		return !strings.Contains(string(stacks[:runtime.Stack(stacks, true)]), "crypto/tls.(*Conn).clientHandshake")
	}, time.Second, 10*time.Millisecond, "TLS handshake goroutine remained parked")
}

func TestIPCTLSHandshakeTimeout(t *testing.T) {
	tests := []struct {
		name                string
		serverTimeout       int
		tlsHandshakeTimeout time.Duration
		expected            time.Duration
	}{
		{
			name:                "uses the configured TLS handshake timeout",
			serverTimeout:       30,
			tlsHandshakeTimeout: 10 * time.Second,
			expected:            10 * time.Second,
		},
		{
			name:                "caps the TLS handshake timeout at the server timeout",
			serverTimeout:       5,
			tlsHandshakeTimeout: 10 * time.Second,
			expected:            5 * time.Second,
		},
		{
			name:                "does not disable the timeout when the TLS setting is zero",
			serverTimeout:       5,
			tlsHandshakeTimeout: 0,
			expected:            5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := config.NewMock(t)
			conf.Set("server_timeout", tt.serverTimeout, pkgconfigmodel.SourceAgentRuntime)
			conf.Set("tls_handshake_timeout", tt.tlsHandshakeTimeout, pkgconfigmodel.SourceAgentRuntime)

			serverTimeout := conf.GetDuration("server_timeout") * time.Second
			assert.Equal(t, tt.expected, ipcTLSHandshakeTimeout(conf, serverTimeout))
		})
	}
}

func TestPostChunk(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)

		for _, chunk := range []string{"chunk1", "chunk2", "chunk3"} {
			_, err := fmt.Fprint(w, chunk)
			require.NoError(t, err)
			flusher.Flush()
		}
	}
	client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)

	var receivedChunks [][]byte
	onChunk := func(chunk []byte) {
		// Copy chunk as the buffer might be reused
		copiedChunk := make([]byte, len(chunk))
		copy(copiedChunk, chunk)
		receivedChunks = append(receivedChunks, copiedChunk)
	}

	err := client.PostChunk(ts.URL, "application/json", nil, onChunk)
	require.NoError(t, err)

	expectedChunks := [][]byte{[]byte("chunk1"), []byte("chunk2"), []byte("chunk3")}
	require.Equal(t, expectedChunks, receivedChunks)
}

func TestNewIPCEndpoint(t *testing.T) {
	// test the endpoint construction
	client, ts := getMockServerAndConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}), testToken)
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
	}), testToken)

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
	}), testToken)

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
	}), testToken)

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
	}), testToken)

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
	}), testToken)

	end, err := client.NewIPCEndpoint("test/api")
	require.NoError(t, err)

	// test that error gets unwrapped from the errmap
	_, err = end.DoGet()
	require.Error(t, err)
	assert.Equal(t, err.Error(), "something went wrong")
}

// Verifies WithoutAuthToken prevents the IPC client from attaching the bearer token.
func TestWithoutAuthToken(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}
	client, ts := getMockServerAndConfig(t, http.HandlerFunc(handler), testToken)

	_, err := client.Get(ts.URL, WithoutAuthToken)
	require.NoError(t, err)
}
