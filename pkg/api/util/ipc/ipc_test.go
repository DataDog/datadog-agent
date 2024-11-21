// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/api/util"
)

type MockResolver struct {
	store map[string][]Endpoint
}

func NewMockResolver() *MockResolver {
	return &MockResolver{
		store: map[string][]Endpoint{},
	}
}

func (m *MockResolver) Resolve(addr string) ([]Endpoint, error) {
	if val, ok := m.store[addr]; ok {
		return val, nil
	}
	return []Endpoint{}, fmt.Errorf("addr %v not found", addr)
}

func (m *MockResolver) AddEndpoints(addr string, endpoints ...Endpoint) {
	m.store[addr] = endpoints
}

type HelloHandler struct {
}

// ServeHTTP serve "hello world"
func (hh *HelloHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("hello world"))
}

type certKeyPair struct {
	cert []byte
	key  []byte
}

type TestCase struct {
	testName          string
	genericName       string
	resolvedEndpoints []Endpoint
	certKey           *certKeyPair
	clientOptions     []ClientOptionCb
	serverOptions     []ServerOptionCb
	expectedOutput    string
}

func TestClientServer(t *testing.T) {
	tests := []TestCase{
		{
			testName:          "basic http test",
			genericName:       "test",
			resolvedEndpoints: []Endpoint{NewTCPEndpoint("127.0.0.1:8080")},
			certKey:           nil,
			clientOptions:     []ClientOptionCb{},
			serverOptions:     []ServerOptionCb{},
			expectedOutput:    "hello world",
		},
		{
			testName:          "basic https test",
			genericName:       "test",
			resolvedEndpoints: []Endpoint{NewTCPEndpoint("127.0.0.1:8081")},
			certKey:           generateSelfSignedCert(t, "test"),
			clientOptions:     []ClientOptionCb{},
			serverOptions:     []ServerOptionCb{},
			expectedOutput:    "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			resolver := NewMockResolver()

			var clientTLSConfig *tls.Config
			var serverTLSConfig *tls.Config

			resolver.AddEndpoints(tt.genericName, tt.resolvedEndpoints...)

			if tt.certKey != nil {
				// Create a TLS certificate using the generated cert and key
				tlsCert, err := tls.X509KeyPair(tt.certKey.cert, tt.certKey.key)
				require.NoError(t, err)

				// Create a certificate pool and add the self-signed certificate
				certPool := x509.NewCertPool()
				ok := certPool.AppendCertsFromPEM(tt.certKey.cert)
				require.True(t, ok)

				serverTLSConfig = &tls.Config{
					Certificates: []tls.Certificate{tlsCert},
				}

				clientTLSConfig = &tls.Config{
					RootCAs: certPool,
				}
			}

			tt.clientOptions = append(tt.clientOptions, WithClientResolver(resolver), WithTLSConfig(clientTLSConfig))
			tt.serverOptions = append(tt.serverOptions, WithServerResolver(resolver))

			s := &http.Server{
				Addr:      tt.genericName,
				Handler:   &HelloHandler{},
				TLSConfig: serverTLSConfig,
			}

			s2, err := NewIPCServer(s, tt.serverOptions...)
			assert.NoError(t, err)

			go func() {
				serveFunc := s2.Serve
				if tt.certKey != nil {
					serveFunc = s2.ServeTLS
				}
				err := serveFunc()
				assert.NoError(t, err)
			}()

			// Allow the server to start
			time.Sleep(1 * time.Second)

			url := url.URL{
				Scheme: func() string {
					if tt.certKey != nil {
						return "https"
					}
					return "http"
				}(),
				Host: tt.genericName,
			}

			client := GetClient(tt.clientOptions...)
			data, err := util.DoGet(client, url.String(), util.CloseConnection)
			require.NoError(t, err)
			require.Equal(t, tt.expectedOutput, string(data))
		})
	}

}

// generateSelfSignedCert generates a self-signed certificate and key.
func generateSelfSignedCert(t *testing.T, genericName string) *certKeyPair {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{genericName},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return &certKeyPair{certPEM, keyPEM}
}
