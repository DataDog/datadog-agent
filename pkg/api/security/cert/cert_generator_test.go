// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertCommunication(t *testing.T) {
	res, err := generateCertKeyPair()
	assert.NoError(t, err)

	// Load server certificate
	serverCert, err := tls.X509KeyPair(res.cert, res.key)
	assert.NoError(t, err)

	// Create a certificate pool with the generated certificate
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(res.cert)
	assert.True(t, ok)

	// Create a TLS config for the server
	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	// Create a TLS config for the client
	clientTLSConfig := &tls.Config{
		RootCAs: certPool,
	}

	expectedResult := []byte("hello word")

	// Create a HTTPS Server
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(expectedResult)
	}))

	s.TLS = serverTLSConfig
	s.StartTLS()
	t.Cleanup(func() {
		s.Close()
	})

	// Create a HTTPS Client
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
	}

	// Try to communicate together
	resp, err := client.Get(s.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, expectedResult)
}
