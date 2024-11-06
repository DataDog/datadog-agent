// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/gorilla/mux"
)

type server struct {
	srv      *http.Server
	listener net.Listener
}

// validateToken - validates token for legacy API
func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newServer(endpoint string, handler http.Handler) (*server, error) {

	// Generate a self-signed certificate
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Datadog Inc."},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range []string{"127.0.0.1", "localhost", "::1"} {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	// parse the resulting certificate so we can use it again
	_, err = x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	// PEM encode the certificate (this is a standard TLS encoding)
	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM := pem.EncodeToMemory(&b)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("unable to generate TLS key pair: %v", err)
	}

	tlsCertPool := x509.NewCertPool()
	ok := tlsCertPool.AppendCertsFromPEM(certPEM)
	if !ok {
		return nil, fmt.Errorf("unable to add new certificate to pool")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS12,
	}

	r := mux.NewRouter()
	r.Handle("/", handler)

	r.Use(validateToken)

	s := &http.Server{
		Addr:      endpoint,
		TLSConfig: tlsConfig,
		Handler:   r,
	}

	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return nil, err
	}

	tlsListener := tls.NewListener(listener, s.TLSConfig)

	return &server{
		srv:      s,
		listener: tlsListener,
	}, nil

}

func (s *server) start() error {
	return s.srv.Serve(s.listener)
}

func (s *server) shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
