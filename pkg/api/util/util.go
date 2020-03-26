// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package util

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/api/security"
)

var (
	token    string
	dcaToken string
)

// SetAuthToken sets the session token
// Requires that the config has been set up before calling
func SetAuthToken() error {
	// Noop if token is already set
	if token != "" {
		return nil
	}

	// token is only set once, no need to mutex protect
	var err error
	token, err = security.FetchAuthToken()
	return err
}

// CreateAndSetAuthToken creates and sets the authorization token
// Requires that the config has been set up before calling
func CreateAndSetAuthToken() error {
	// Noop if token is already set
	if token != "" {
		return nil
	}

	// token is only set once, no need to mutex protect
	var err error
	token, err = security.CreateOrFetchToken()
	return err
}

// GetAuthToken gets the session token
func GetAuthToken() string {
	return token
}

// InitDCAAuthToken initialize the session token for the Cluster Agent based on config options
// Requires that the config has been set up before calling
func InitDCAAuthToken() error {
	// Noop if dcaToken is already set
	if dcaToken != "" {
		return nil
	}

	// dcaToken is only set once, no need to mutex protect
	var err error
	dcaToken, err = security.CreateOrGetClusterAgentAuthToken()
	return err
}

// GetDCAAuthToken gets the session token
func GetDCAAuthToken() string {
	return dcaToken
}

// Validate validates an http request
func Validate(w http.ResponseWriter, r *http.Request) error {
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

	if len(tok) < 2 || tok[1] != GetAuthToken() {
		err = fmt.Errorf("invalid session token")
		http.Error(w, err.Error(), 403)
	}

	return err
}

// ValidateDCARequest is used for the exposed endpoints of the DCA.
// It is different from Validate as we want to have different validations.
func ValidateDCARequest(w http.ResponseWriter, r *http.Request) error {
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

	if len(tok) != 2 || tok[1] != GetDCAAuthToken() {
		err = fmt.Errorf("invalid session token")
		http.Error(w, err.Error(), 403)
	}

	return err
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

// IsIPv6 is used to differentiate between ipv4 and ipv6 addresses based on colons.
// The function does NOT verify if the argument string is a valid address
func IsIPv6(ip string) bool {
	return strings.Count(ip, ":") >= 2
}
