// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util implements helper functions for the api
package util

import (
	"net/http"
)

var (
	token    string
	dcaToken string
)

// SetAuthToken sets the session token
// Requires that the config has been set up before calling
func SetAuthToken() error {
	panic("not called")
}

// CreateAndSetAuthToken creates and sets the authorization token
// Requires that the config has been set up before calling
func CreateAndSetAuthToken() error {
	panic("not called")
}

// GetAuthToken gets the session token
func GetAuthToken() string {
	panic("not called")
}

// InitDCAAuthToken initialize the session token for the Cluster Agent based on config options
// Requires that the config has been set up before calling
func InitDCAAuthToken() error {
	panic("not called")
}

// GetDCAAuthToken gets the session token
func GetDCAAuthToken() string {
	panic("not called")
}

// Validate validates an http request
func Validate(w http.ResponseWriter, r *http.Request) error {
	panic("not called")
}

// ValidateDCARequest is used for the exposed endpoints of the DCA.
// It is different from Validate as we want to have different validations.
func ValidateDCARequest(w http.ResponseWriter, r *http.Request) error {
	panic("not called")
}

// IsForbidden returns whether the cluster check runner server is allowed to listen on a given ip
// The function is a non-secure helper to help avoiding setting an IP that's too permissive.
// The function doesn't guarantee any security feature
func IsForbidden(ip string) bool {
	panic("not called")
}

// IsIPv6 is used to differentiate between ipv4 and ipv6 addresses.
func IsIPv6(ip string) bool {
	panic("not called")
}
