// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2ecoverage

// Package apiutil provides utility functions for the API.
package apiutil

import "net/http"

// SetupCoverageHandler does nothing when compiling without the e2ecoverage build tag
func SetupCoverageHandler(_ *http.ServeMux) {}
