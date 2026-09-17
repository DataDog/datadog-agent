// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package oci

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

const testDigest = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"

func TestNewSource(t *testing.T) {
	t.Run("requires environment", func(t *testing.T) {
		source, err := NewSource(nil, http.DefaultClient)
		require.ErrorContains(t, err, "environment is required")
		assert.Nil(t, source)
	})

	t.Run("requires client", func(t *testing.T) {
		source, err := NewSource(&installerenv.Env{}, nil)
		require.ErrorContains(t, err, "HTTP client is required")
		assert.Nil(t, source)
	})

	t.Run("variant includes platform and flavor", func(t *testing.T) {
		source, err := NewSource(&installerenv.Env{FIPSMode: true}, http.DefaultClient)
		require.NoError(t, err)
		assert.Equal(t, strings.Join([]string{materializationLayoutVersion, runtime.GOOS, runtime.GOARCH, "fips"}, "-"), source.Variant())
	})
}

func TestSourceFetch(t *testing.T) {
	server := fixtures.NewServer(t)
	packageURL := server.PackageURL(fixtures.FixtureSimpleV1)
	digest := packageURL[strings.LastIndex(packageURL, "@sha256:")+len("@sha256:"):]
	source, err := NewSource(&installerenv.Env{}, server.Client())
	require.NoError(t, err)
	destination := t.TempDir()

	err = source.Fetch(context.Background(), authoredscripts.Descriptor{
		Package: fixtures.FixtureSimpleV1.Package,
		Version: fixtures.FixtureSimpleV1.Version,
		URL:     packageURL,
		SHA256:  digest,
	}, destination)

	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "executable.sh"))
	require.NoError(t, err)
}

func TestSourceFetchRejectsPackageMetadataBeforeExtraction(t *testing.T) {
	server := fixtures.NewServer(t)
	packageURL := server.PackageURL(fixtures.FixtureSimpleV1)
	digest := packageURL[strings.LastIndex(packageURL, "@sha256:")+len("@sha256:"):]
	source, err := NewSource(&installerenv.Env{}, server.Client())
	require.NoError(t, err)
	destination := t.TempDir()

	err = source.Fetch(context.Background(), authoredscripts.Descriptor{
		Package: "other-package",
		Version: fixtures.FixtureSimpleV1.Version,
		URL:     packageURL,
		SHA256:  digest,
	}, destination)

	require.ErrorContains(t, err, "does not match catalog package")
	entries, readErr := os.ReadDir(destination)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestValidateReference(t *testing.T) {
	tests := []struct {
		name       string
		descriptor authoredscripts.Descriptor
		wantError  string
	}{
		{
			name: "valid",
			descriptor: authoredscripts.Descriptor{
				URL:    "oci://registry.example.test/package@sha256:" + testDigest,
				SHA256: testDigest,
			},
		},
		{
			name: "unsupported scheme",
			descriptor: authoredscripts.Descriptor{
				URL:    "https://registry.example.test/package@sha256:" + testDigest,
				SHA256: testDigest,
			},
			wantError: "unsupported scheme",
		},
		{
			name: "mutable reference",
			descriptor: authoredscripts.Descriptor{
				URL:    "oci://registry.example.test/package:latest",
				SHA256: testDigest,
			},
			wantError: "immutable OCI digest",
		},
		{
			name: "digest mismatch",
			descriptor: authoredscripts.Descriptor{
				URL:    "oci://registry.example.test/package@sha256:" + testDigest,
				SHA256: strings.Repeat("b", 64),
			},
			wantError: "does not match expected digest",
		},
		{
			name: "userinfo",
			descriptor: authoredscripts.Descriptor{
				URL:    "oci://user:password@registry.example.test/package@sha256:" + testDigest,
				SHA256: testDigest,
			},
			wantError: "must not contain user information",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateReference(test.descriptor)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
