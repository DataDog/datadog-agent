// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import "errors"

var ErrPackageNotConfigured = errors.New("authored-script package is not configured")

// Descriptor identifies an immutable published artifact variant.
type Descriptor struct {
	Package string
	Version string
	URL     string
	SHA256  string
}

type Catalog interface {
	Lookup(key string) (Descriptor, error)
}
