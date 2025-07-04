// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.

//go:build !e2ecoverage

// Package coverage does nothing when compiling without the e2ecoverage build tag.
package coverage

import "github.com/gorilla/mux"

// SetupCoverageHandler does nothing when compiling without the e2ecoverage build tag
func SetupCoverageHandler(_ *mux.Router) {}
