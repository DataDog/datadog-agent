// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2ecoverage

// Package apiutil provides utility functions for the API.
package apiutil

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/coverage"
)

// SetupCoverageHandler adds the coverage handler to the router
func SetupCoverageHandler(r *http.ServeMux) {
	r.HandleFunc("/coverage", coverage.ComponentCoverageHandler)
}
