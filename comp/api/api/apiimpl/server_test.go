// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
)

func TestAuthTagGetter(t *testing.T) {
	ipcComp := ipcmock.New(t)

	testCases := []struct {
		name              string
		serverTLSConfig   *tls.Config
		clientTLSConfig   *tls.Config
		authTagShouldFail bool
		expectTLSFailure  bool // with strict mTLS, no/wrong client cert fails at handshake
		expectedTag       string
	}{
		{
			name:              "secure server & secure client",
			serverTLSConfig:   ipcComp.GetTLSServerConfig(),
			clientTLSConfig:   ipcComp.GetTLSClientConfig(),
			authTagShouldFail: false,
			expectedTag:       "mTLS",
		},
		{
			name:              "secure server & insecure client",
			serverTLSConfig:   ipcComp.GetTLSServerConfig(),
			clientTLSConfig:   &tls.Config{InsecureSkipVerify: true},
			expectTLSFailure:  true, // server requires client cert
			authTagShouldFail: false,
			expectedTag:       "token",
		},
		{
			name:              "insecure server & secure client",
			serverTLSConfig:   nil,
			clientTLSConfig:   ipcComp.GetTLSClientConfig(),
			authTagShouldFail: true,
			expectedTag:       "token",
		},
		{
			name:              "insecure server & insecure client",
			serverTLSConfig:   nil,
			clientTLSConfig:   &tls.Config{InsecureSkipVerify: true},
			authTagShouldFail: true,
			expectedTag:       "token",
		},
		{
			name:            "secure server & secure client with different certificate",
			serverTLSConfig: ipcComp.GetTLSServerConfig(),
			clientTLSConfig: func() *tls.Config {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				cfg := &tls.Config{
					InsecureSkipVerify: true,
					Certificates:       []tls.Certificate{server.TLS.Certificates[0]},
				}
				return cfg
			}(),
			expectTLSFailure:  true, // server rejects cert from different CA
			authTagShouldFail: false,
			expectedTag:       "token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authTagGetter, err := authTagGetter(tc.serverTLSConfig)
			if tc.authTagShouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			var tcHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
				tag := authTagGetter(r)
				assert.Equal(t, tc.expectedTag, tag)
				w.WriteHeader(http.StatusOK)
			}

			server := ipcComp.NewMockServer(tcHandler)

			url := url.URL{
				Scheme: "https",
				Host:   server.Listener.Addr().String(),
				Path:   "/",
			}

			req, err := http.NewRequest(http.MethodGet, url.String(), nil)
			require.NoError(t, err)

			client := server.Client()
			client.Transport.(*http.Transport).TLSClientConfig = tc.clientTLSConfig

			resp, err := client.Do(req)
			if tc.expectTLSFailure {
				require.Error(t, err, "expected TLS handshake failure when client does not present valid cert")
				return
			}
			require.NoError(t, err)
			resp.Body.Close()
		})
	}
}
