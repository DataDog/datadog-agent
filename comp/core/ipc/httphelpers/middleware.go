// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package httphelpers

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// NewHTTPMiddleware returns a middleware that validates the auth token for the given request
func NewHTTPMiddleware(logger func(format string, params ...interface{}), authtoken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
				err = errors.New("no session token provided")
				http.Error(w, err.Error(), 401)
				logger("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
				return
			}

			tok := strings.Split(auth, " ")
			if tok[0] != "Bearer" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
				err = fmt.Errorf("unsupported authorization scheme: %s", tok[0])
				http.Error(w, err.Error(), 401)
				logger("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)

				return
			}

			// The following comparison must be evaluated in constant time
			if len(tok) < 2 || !constantCompareStrings(tok[1], authtoken) {
				err = errors.New("invalid session token")
				http.Error(w, err.Error(), 403)
				logger("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// constantCompareStrings compares two strings in constant time.
// It uses the subtle.ConstantTimeCompare function from the crypto/subtle package
// to compare the byte slices of the input strings.
// Returns true if the strings are equal, false otherwise.
func constantCompareStrings(src, tgt string) bool {
	return subtle.ConstantTimeCompare([]byte(src), []byte(tgt)) == 1
}
