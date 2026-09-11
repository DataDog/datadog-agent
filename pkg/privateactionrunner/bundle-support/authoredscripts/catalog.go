// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
)

var ErrPackageNotConfigured = errors.New("authored-script package is not configured")

// Descriptor identifies an immutable published artifact variant.
type Descriptor struct {
	Package string
	Version string
	URL     string
	SHA256  string
}

// Validate checks that the descriptor contains valid artifact coordinates.
func (d Descriptor) Validate() error {
	if d.Package == "" {
		return errors.New("authored-script package is required")
	}
	if d.Version == "" {
		return errors.New("authored-script version is required")
	}
	if d.URL == "" {
		return errors.New("authored-script URL is required")
	}
	if d.SHA256 == "" {
		return errors.New("authored-script SHA-256 digest is required")
	}

	artifactDigest := digest.NewDigestFromEncoded(digest.SHA256, d.SHA256)
	if err := artifactDigest.Validate(); err != nil {
		return fmt.Errorf("invalid authored-script SHA-256 digest %q: %w", d.SHA256, err)
	}
	return nil
}

type Catalog interface {
	Lookup(key string) (Descriptor, error)
}
