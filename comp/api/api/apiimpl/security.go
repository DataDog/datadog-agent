// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// validateToken - validates token for legacy API
func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			log.Warnf("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
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

func buildSelfSignedKeyPair(additionalHostIdentities ...string) ([]byte, []byte) {
	hosts := []string{"127.0.0.1", "localhost", "::1"}
	hosts = append(hosts, additionalHostIdentities...)
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

func initializeTLS(additionalHostIdentities ...string) (*tls.Certificate, *x509.CertPool, error) {
	// print the caller to identify what is calling this function
	if _, file, line, ok := runtime.Caller(1); ok {
		log.Infof("[%s:%d] Initializing TLS certificates for hosts %v", file, line, strings.Join(additionalHostIdentities, ", "))
	}

	cert, key := buildSelfSignedKeyPair(additionalHostIdentities...)
	if cert == nil {
		return nil, nil, errors.New("unable to generate certificate")
	}
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate TLS key pair: %v", err)
	}

	tlsCertPool := x509.NewCertPool()
	ok := tlsCertPool.AppendCertsFromPEM(cert)
	if !ok {
		return nil, nil, fmt.Errorf("unable to add new certificate to pool")
	}

	return &pair, tlsCertPool, nil
}
