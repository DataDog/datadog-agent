// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package common

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var token string

// SetSessionToken sets the session token
func SetSessionToken() error {
	if token != "" {
		return fmt.Errorf("session token already set")
	}

	// token is only set once, no need to mutex protect
	token = config.Datadog.GetString("api_key") // FIXME: encode this into JWT?
	return nil
}

// GetSessionToken gets the session token
func GetSessionToken() string {
	// FIXME: make this a real session id
	return token
}

// Validate validates an http request
func Validate(r *http.Request) error {
	tok := r.Header.Get("Session-Token")
	if tok == "" {
		return fmt.Errorf("no session token available")
	}

	if tok != GetSessionToken() {
		return fmt.Errorf("invalid session token")
	}

	return nil
}
