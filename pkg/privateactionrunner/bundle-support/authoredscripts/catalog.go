// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/go-digest"
)

var ErrPackageNotConfigured = errors.New("authored-script package is not configured")

// Descriptor identifies an authored script and its immutable published artifact variant.
type Descriptor struct {
	FQN     string
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

	parsedURL, err := url.Parse(d.URL)
	if err != nil {
		return fmt.Errorf("could not parse authored-script OCI URL: %w", err)
	}
	if parsedURL.Scheme != "oci" {
		return fmt.Errorf("authored-script package URL uses unsupported scheme %q", parsedURL.Scheme)
	}
	if parsedURL.User != nil {
		return errors.New("authored-script OCI URL must not contain user information")
	}
	if parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return errors.New("authored-script OCI URL must not contain a query or fragment")
	}

	reference, err := name.NewDigest(strings.TrimPrefix(d.URL, "oci://"), name.StrictValidation)
	if err != nil {
		return fmt.Errorf("authored-script package URL must contain a valid immutable OCI digest: %w", err)
	}
	expectedDigest := "sha256:" + d.SHA256
	if reference.DigestStr() != expectedDigest {
		return fmt.Errorf("authored-script OCI reference digest %q does not match expected digest %q", reference.DigestStr(), expectedDigest)
	}
	return nil
}

type Catalog interface {
	Lookup(fqn string) (Descriptor, error)
}
