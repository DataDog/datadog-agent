// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact"
)

var ErrPackageNotConfigured = errors.New("authored-script package is not configured")

// Descriptor identifies an immutable published artifact variant.
type Descriptor struct {
	Package  string
	Version  string
	URL      string
	SHA256   string
	Size     int64
	Platform artifact.Platform
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
	if d.Size < 0 {
		return errors.New("authored-script artifact size cannot be negative")
	}
	if (d.Platform.OS == "") != (d.Platform.Architecture == "") {
		return errors.New("authored-script platform OS and architecture must be set together")
	}
	if d.Platform.Variant != "" && d.Platform.OS == "" {
		return errors.New("authored-script platform variant requires an OS and architecture")
	}
	return nil
}

func descriptorFromArtifact(value artifact.Descriptor) Descriptor {
	return Descriptor{
		Package:  value.Package,
		Version:  value.Version,
		URL:      value.Reference,
		SHA256:   value.Digest.Encoded(),
		Size:     value.Size,
		Platform: value.Platform,
	}
}

func (d Descriptor) artifactDescriptor(platform artifact.Platform) (artifact.Descriptor, error) {
	if err := d.Validate(); err != nil {
		return artifact.Descriptor{}, err
	}
	selectedPlatform := platform
	if d.Platform.OS != "" {
		if d.Platform.OS != platform.OS || d.Platform.Architecture != platform.Architecture {
			return artifact.Descriptor{}, fmt.Errorf("authored-script catalog platform %s/%s does not match downloader platform %s/%s", d.Platform.OS, d.Platform.Architecture, platform.OS, platform.Architecture)
		}
		selectedPlatform.OS = d.Platform.OS
		selectedPlatform.Architecture = d.Platform.Architecture
	}
	return artifact.Descriptor{
		Package:   strings.ToLower(d.Package),
		Version:   d.Version,
		Reference: d.URL,
		Digest:    digest.NewDigestFromEncoded(digest.SHA256, d.SHA256),
		Size:      d.Size,
		Platform:  selectedPlatform,
	}, nil
}

func (d Descriptor) catalogDescriptor() (artifact.Descriptor, error) {
	if err := d.Validate(); err != nil {
		return artifact.Descriptor{}, err
	}
	return artifact.Descriptor{
		Package:   strings.ToLower(d.Package),
		Version:   d.Version,
		Reference: d.URL,
		Digest:    digest.NewDigestFromEncoded(digest.SHA256, d.SHA256),
		Size:      d.Size,
		Platform:  d.Platform,
	}, nil
}

// Catalog resolves authored-script actions and rechecks authorization at the
// point where execution starts.
type Catalog interface {
	Lookup(key string) (Descriptor, error)
	WithAuthorized(descriptor Descriptor, use func() error) error
}
