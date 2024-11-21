// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package utils contains common code shared across the USM codebase
package utils

import "net/http"

// TracedProgramsEndpoint is not supported on Windows
func TracedProgramsEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(404)
}

// BlockedPathIDEndpoint is not supported on Windows
func BlockedPathIDEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(404)
}

// ClearBlockedEndpoint is not supported on Windows
func ClearBlockedEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(404)
}

// AttachPIDEndpoint is not supported on Windows
func AttachPIDEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(404)
}

// DetachPIDEndpoint is not supported on Windows
func DetachPIDEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(404)
}
