// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
)

// AuthTagGetter returns a function that returns the auth tag for the given request.
// It returns "mTLS" if the client provides the IPC certificate, "token" otherwise.
//
// The returned tag is meant to be used as the value of the "auth" label of the
// api_server_request_duration_seconds metric, to track whether the clients of an
// agent process's IPC API support mTLS or only token authentication.
func AuthTagGetter(serverTLSConfig *tls.Config) (func(r *http.Request) string, error) {
	// Read the IPC certificate from the server TLS config
	if serverTLSConfig == nil || len(serverTLSConfig.Certificates) == 0 || len(serverTLSConfig.Certificates[0].Certificate) == 0 {
		return nil, errors.New("no certificates found in server TLS config")
	}

	cert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("error parsing IPC certificate: %v", err)
	}

	return func(r *http.Request) string {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 && cert.Equal(r.TLS.PeerCertificates[0]) {
			return "mTLS"
		}
		// We can assert that the auth is at least a token because it has been checked previously by the validateToken middleware
		return "token"
	}, nil
}

// NoTLSAuthTagGetter returns an auth tag getter for servers that do not use TLS
// (e.g. plain HTTP over a unix socket), where mTLS cannot be negotiated.
func NoTLSAuthTagGetter() func(r *http.Request) string {
	return func(*http.Request) string { return "no_tls" }
}
