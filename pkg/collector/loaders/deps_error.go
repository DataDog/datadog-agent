// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loaders

import (
	"strings"
)

// DepsPendingError is returned by a loader when a check could not be loaded
// because some of its declared third-party dependencies are not installed yet.
// Schedulers can use it to defer the check and trigger a background
// installation of the missing packages instead of reporting a load failure.
type DepsPendingError struct {
	// Deps holds the names of the missing dependency packages.
	Deps []string
}

// NewDepsPendingError returns a DepsPendingError for the given missing deps.
func NewDepsPendingError(deps []string) *DepsPendingError {
	return &DepsPendingError{Deps: deps}
}

// Error implements the error interface.
func (e *DepsPendingError) Error() string {
	return "missing dependency packages: " + strings.Join(e.Deps, ", ")
}
