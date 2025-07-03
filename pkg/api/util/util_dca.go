// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util implements helper functions for the api
package util

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

var (
	tokenLock sync.RWMutex
	dcaToken  string
)

// InitDCAAuthToken initialize the session token for the Cluster Agent based on config options
// Requires that the config has been set up before calling
func InitDCAAuthToken(config model.Reader) error {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	// Noop if dcaToken is already set
	if dcaToken != "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.GetDuration("auth_init_timeout"))
	defer cancel()

	var err error
	dcaToken, err = pkgtoken.CreateOrGetClusterAgentAuthToken(ctx, config)
	return err
}

// GetDCAAuthToken gets the session token
func GetDCAAuthToken() string {
	tokenLock.RLock()
	defer tokenLock.RUnlock()
	return dcaToken
}

// TokenValidator is a middleware that validates the session token for the DCA.
// It checks the "Authorization" header for a Bearer token and compares it to the
// session token stored in the configuration.
func TokenValidator(tokenGetter func() string) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var err error
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
			err = fmt.Errorf("no session token provided")
			http.Error(w, err.Error(), 401)
			return err
		}

		tok := strings.Split(auth, " ")
		if tok[0] != "Bearer" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
			err = fmt.Errorf("unsupported authorization scheme: %s", tok[0])
			http.Error(w, err.Error(), 401)
			return err
		}

		// The following comparison must be evaluated in constant time
		if len(tok) != 2 || !constantCompareStrings(tok[1], tokenGetter()) {
			err = fmt.Errorf("invalid session token")
			http.Error(w, err.Error(), 403)
		}

		return err
	}
}

// constantCompareStrings compares two strings in constant time.
// It uses the subtle.ConstantTimeCompare function from the crypto/subtle package
// to compare the byte slices of the input strings.
// Returns true if the strings are equal, false otherwise.
func constantCompareStrings(src, tgt string) bool {
	return subtle.ConstantTimeCompare([]byte(src), []byte(tgt)) == 1
}

// IsForbidden returns whether the cluster check runner server is allowed to listen on a given ip
// The function is a non-secure helper to help avoiding setting an IP that's too permissive.
// The function doesn't guarantee any security feature
func IsForbidden(ip string) bool {
	forbidden := map[string]bool{
		"":                true,
		"0.0.0.0":         true,
		"::":              true,
		"0:0:0:0:0:0:0:0": true,
	}
	return forbidden[ip]
}

// IsIPv6 is used to differentiate between ipv4 and ipv6 addresses.
func IsIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}
