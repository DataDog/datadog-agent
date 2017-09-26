// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package common

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var token string

// SetAuthToken sets the session token
func SetAuthToken() error {
	if token != "" {
		return fmt.Errorf("session token already set")
	}

	// token is only set once, no need to mutex protect
	token = config.Datadog.GetString("api_key") // FIXME: encode this into JWT?
	return nil
}

// GetAuthToken gets the session token
func GetAuthToken() string {
	return token
}

// Validate validates an http request
func Validate(w http.ResponseWriter, r *http.Request) (err error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"Datadog Agent\"")
		err = fmt.Errorf("no session token provided")
		http.Error(w, err.Error(), 401)
		return
	}

	tok := strings.Split(auth, " ")
	if tok[0] != "Bearer" {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"Datadog Agent\"")
		err = fmt.Errorf("Unsupported authorization scheme: %s", tok[0])
		http.Error(w, err.Error(), 401)
		return
	}

	if len(tok) < 2 || tok[1] != GetAuthToken() {
		err = fmt.Errorf("invalid session token")
		http.Error(w, err.Error(), 403)
	}

	return
}
