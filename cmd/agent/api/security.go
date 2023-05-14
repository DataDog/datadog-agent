// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
)

var (
	tlsKeyPair  *tls.Certificate
	tlsCertPool *x509.CertPool
	tlsAddr     string
)

// validateToken - validates token for legacy API
func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseToken parses the token and validate it for our gRPC API, it returns an empty
// struct and an error or nil
func parseToken(token string) (interface{}, error) {
	if token != util.GetAuthToken() {
		return struct{}{}, errors.New("Invalid session token")
	}

	// Currently this empty struct doesn't add any information
	// to the context, but we could potentially add some custom
	// type.
	return struct{}{}, nil
}

func buildSelfSignedKeyPair() ([]byte, []byte) {

	hosts := []string{"127.0.0.1", "localhost", "::1"}
	ipcAddr, err := getIPCAddressPort()
	if err == nil {
		hosts = append(hosts, ipcAddr)
	}
	_, rootCertPEM, rootKey, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return nil, nil
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create and return TLS private cert and key
	return rootCertPEM, rootKeyPEM
}

func initializeTLS() {

	cert, key := buildSelfSignedKeyPair()
	if cert == nil {
		panic("unable to generate certificate")
	}
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		panic(err)
	}
	tlsKeyPair = &pair
	tlsCertPool = x509.NewCertPool()
	ok := tlsCertPool.AppendCertsFromPEM(cert)
	if !ok {
		panic("bad certs")
	}

	tlsAddr, err = getIPCAddressPort()
	if err != nil {
		panic("unable to get IPC address and port")
	}
}
