// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auth

import (
	"bytes"
	"io"
	"net/http"
)

// GetHTTPGuardMiddleware provides an HTTP middleware that verifies the authorization of incoming requests.
func GetHTTPGuardMiddleware(a Authorizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			statusCode, err := a.VerifyREST(r.Method, r.Header, r.Body, r.ContentLength)

			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
				http.Error(w, err.Error(), statusCode)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetSecureRoundTripper provides a transport wrapper that sign outgoing requests.
func GetSecureRoundTripper(a Authorizer, r http.RoundTripper) http.RoundTripper {
	return secureRoundTripper{
		Transport:     r,
		authenticator: a,
	}
}

type secureRoundTripper struct {
	Transport     http.RoundTripper
	authenticator Authorizer
}

func (s secureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read and restore the request body (required since reading it consumes it)
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Compute the signature
	err := s.authenticator.SignREST(req.Method, req.Header, req.Body, req.ContentLength)
	if err != nil {
		return nil, err
	}

	// Use the provided Transport (default if nil)
	if s.Transport == nil {
		s.Transport = http.DefaultTransport
	}
	return s.Transport.RoundTrip(req)
}
