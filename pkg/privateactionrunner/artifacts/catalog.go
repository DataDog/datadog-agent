// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifacts

import "errors"

var ErrNotFound = errors.New("artifact not found")

// Platform identifies the target operating system and CPU architecture for an artifact.
type Platform struct {
	OS   string
	Arch string
}

// Descriptor identifies an immutable published artifact variant.
type Descriptor struct {
	Name     string
	Version  string
	URL      string
	Digest   string
	Platform Platform
}

// Catalog finds the authorized artifact for a logical key and platform.
type Catalog interface {
	Lookup(key string, platform Platform) (Descriptor, error)
}
