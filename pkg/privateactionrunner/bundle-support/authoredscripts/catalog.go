// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import "errors"

// ErrPackageNotConfigured is returned when an authored-script package is not in the catalog.
var ErrPackageNotConfigured = errors.New("authored-script package is not configured")

// Descriptor identifies an immutable published artifact variant.
type Descriptor struct {
	Package string `json:"package"`
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

// Catalog finds the authorized artifact for an authored-script key.
type Catalog interface {
	Lookup(key string) (Descriptor, error)
}
