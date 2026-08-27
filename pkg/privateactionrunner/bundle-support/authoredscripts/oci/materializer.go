// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package oci adapts the Fleet OCI downloader to authored-script package
// materialization.
package oci

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

const materializationLayoutVersion = "datadog-package-v1"

// Materializer downloads authored-script OCI packages and extracts their main
// Datadog Package layer into an artifact-store staging directory.
type Materializer struct {
	downloader   *fleetoci.Downloader
	cacheVariant string
}

// NewMaterializer creates an OCI materializer using the Fleet installer OCI
// implementation. environment controls registry, proxy, platform flavor, and
// authentication behavior; client performs the registry requests.
func NewMaterializer(environment *installerenv.Env, client *http.Client) (*Materializer, error) {
	if environment == nil {
		return nil, errors.New("installer environment is required for authored-script OCI downloads")
	}
	if client == nil {
		return nil, errors.New("HTTP client is required for authored-script OCI downloads")
	}

	// Keep platform selection and the cache variant consistent even if the
	// caller later reuses or modifies its environment value.
	environmentCopy := *environment
	environmentCopy.RegistryOverrideByImage = maps.Clone(environment.RegistryOverrideByImage)
	environmentCopy.RegistryAuthOverrideByImage = maps.Clone(environment.RegistryAuthOverrideByImage)
	environmentCopy.RegistryUsernameByImage = maps.Clone(environment.RegistryUsernameByImage)
	environmentCopy.RegistryPasswordByImage = maps.Clone(environment.RegistryPasswordByImage)
	flavor := "base"
	if environmentCopy.FIPSMode {
		flavor = fleetoci.VariantFIPS
	}
	return &Materializer{
		downloader: fleetoci.NewDownloader(&environmentCopy, client),
		cacheVariant: strings.Join([]string{
			materializationLayoutVersion,
			runtime.GOOS,
			runtime.GOARCH,
			flavor,
		}, "-"),
	}, nil
}

// CacheVariant identifies the platform, flavor, and extracted layout produced
// by this materializer.
func (m *Materializer) CacheVariant() string {
	if m == nil {
		return ""
	}
	return m.cacheVariant
}

// Materialize downloads descriptor's immutable OCI reference and extracts its
// main package layer. Archive extraction remains in pkg/fleet/installer/oci so
// its media-type handling, size limit, retries, and cleanup behavior are reused.
func (m *Materializer) Materialize(
	ctx context.Context,
	descriptor authoredscripts.Descriptor,
	destination string,
) (authoredscripts.MaterializedPackage, error) {
	if ctx == nil {
		return authoredscripts.MaterializedPackage{}, errors.New("authored-script OCI download context is required")
	}
	if m == nil || m.downloader == nil {
		return authoredscripts.MaterializedPackage{}, errors.New("authored-script OCI materializer is not configured")
	}
	if destination == "" {
		return authoredscripts.MaterializedPackage{}, errors.New("authored-script OCI destination is required")
	}
	if err := validateReference(descriptor); err != nil {
		return authoredscripts.MaterializedPackage{}, err
	}

	downloadedPackage, err := m.downloader.Download(ctx, descriptor.URL)
	if err != nil {
		return authoredscripts.MaterializedPackage{}, fmt.Errorf("could not download authored-script OCI package: %w", err)
	}
	if downloadedPackage == nil {
		return authoredscripts.MaterializedPackage{}, errors.New("authored-script OCI downloader returned no package")
	}
	if err := downloadedPackage.ExtractLayers(ctx, fleetoci.DatadogPackageLayerMediaType, destination); err != nil {
		return authoredscripts.MaterializedPackage{}, fmt.Errorf("could not extract authored-script OCI package: %w", err)
	}
	return authoredscripts.MaterializedPackage{
		Package: downloadedPackage.Name,
		Version: downloadedPackage.Version,
	}, nil
}

func validateReference(descriptor authoredscripts.Descriptor) error {
	parsedURL, err := url.Parse(descriptor.URL)
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

	rawReference := strings.TrimPrefix(descriptor.URL, "oci://")
	reference, err := name.NewDigest(rawReference, name.StrictValidation)
	if err != nil {
		return fmt.Errorf("authored-script package URL must contain a valid immutable OCI digest: %w", err)
	}
	expectedDigest := "sha256:" + descriptor.SHA256
	if reference.DigestStr() != expectedDigest {
		return fmt.Errorf("authored-script OCI reference digest %q does not match expected digest %q", reference.DigestStr(), expectedDigest)
	}
	return nil
}
