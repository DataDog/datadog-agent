// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package util

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/api/security"
)

var token string

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

// GetAuthToken gets the session token
func GetAuthToken() string {
	return token
}

// Validate validates an http request
func Validate(w http.ResponseWriter, r *http.Request) error {
	var err error
	auth := r.Header.Get("Authorization")
	if auth == "" {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"Datadog Agent\"")
		err = fmt.Errorf("no session token provided")
		http.Error(w, err.Error(), 401)
		return err
	}

	tok := strings.Split(auth, " ")
	if tok[0] != "Bearer" {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"Datadog Agent\"")
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
