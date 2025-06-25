// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apiutil provides utility functions for the API.
//go:build !e2ecoverage

package apiutil

import "net/http"

func SetupCoverageHandler(r *http.ServeMux) {}
