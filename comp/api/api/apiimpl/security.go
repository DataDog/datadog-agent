// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/subtle"
	"errors"
	"net/http"

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
	if subtle.ConstantTimeCompare([]byte(token), []byte(util.GetAuthToken())) == 0 {
		return struct{}{}, errors.New("Invalid session token")
	}

	// Currently this empty struct doesn't add any information
	// to the context, but we could potentially add some custom
	// type.
	return struct{}{}, nil
}
