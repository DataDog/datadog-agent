// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package oci provides an OCI-backed authored-script package source.
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

// Source materializes authored-script packages from OCI images.
type Source struct {
	downloader *fleetoci.Downloader
	variant    string
}

// NewSource creates an OCI package source using the Fleet downloader.
func NewSource(environment *installerenv.Env, client *http.Client) (*Source, error) {
	if environment == nil {
		return nil, errors.New("installer environment is required for authored-script OCI downloads")
	}
	if client == nil {
		return nil, errors.New("HTTP client is required for authored-script OCI downloads")
	}

	environmentCopy := *environment
	environmentCopy.RegistryOverrideByImage = maps.Clone(environment.RegistryOverrideByImage)
	environmentCopy.RegistryAuthOverrideByImage = maps.Clone(environment.RegistryAuthOverrideByImage)
	environmentCopy.RegistryUsernameByImage = maps.Clone(environment.RegistryUsernameByImage)
	environmentCopy.RegistryPasswordByImage = maps.Clone(environment.RegistryPasswordByImage)
	flavor := "base"
	if environmentCopy.FIPSMode {
		flavor = fleetoci.VariantFIPS
	}
	return &Source{
		downloader: fleetoci.NewDownloader(&environmentCopy, client),
		variant: strings.Join([]string{
			materializationLayoutVersion,
			runtime.GOOS,
			runtime.GOARCH,
			flavor,
		}, "-"),
	}, nil
}

// Variant identifies the platform, flavor, and extracted layout.
func (s *Source) Variant() string {
	if s == nil {
		return ""
	}
	return s.variant
}

// Fetch downloads and extracts the main Datadog Package layer.
func (s *Source) Fetch(ctx context.Context, descriptor authoredscripts.Descriptor, destination string) error {
	if ctx == nil {
		return errors.New("authored-script OCI fetch context is required")
	}
	if s == nil || s.downloader == nil {
		return errors.New("authored-script OCI source is not configured")
	}
	if destination == "" {
		return errors.New("authored-script OCI destination is required")
	}
	if err := validateReference(descriptor); err != nil {
		return err
	}

	downloadedPackage, err := s.downloader.Download(ctx, descriptor.URL)
	if err != nil {
		return fmt.Errorf("could not download authored-script OCI package: %w", err)
	}
	if downloadedPackage == nil {
		return errors.New("authored-script OCI downloader returned no package")
	}
	if downloadedPackage.Name != descriptor.Package {
		return fmt.Errorf("OCI package name %q does not match catalog package %q", downloadedPackage.Name, descriptor.Package)
	}
	if downloadedPackage.Version != descriptor.Version {
		return fmt.Errorf("OCI package version %q does not match catalog version %q", downloadedPackage.Version, descriptor.Version)
	}
	if err := downloadedPackage.ExtractLayers(ctx, fleetoci.DatadogPackageLayerMediaType, destination); err != nil {
		return fmt.Errorf("could not extract authored-script OCI package: %w", err)
	}
	return nil
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
