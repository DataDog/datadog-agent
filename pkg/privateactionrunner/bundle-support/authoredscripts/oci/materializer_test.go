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

func TestNewMaterializer(t *testing.T) {
	t.Run("requires environment", func(t *testing.T) {
		materializer, err := NewMaterializer(nil, http.DefaultClient)
		require.ErrorContains(t, err, "environment is required")
		assert.Nil(t, materializer)
	})

	t.Run("requires client", func(t *testing.T) {
		materializer, err := NewMaterializer(&installerenv.Env{}, nil)
		require.ErrorContains(t, err, "HTTP client is required")
		assert.Nil(t, materializer)
	})

	t.Run("variant includes platform and flavor", func(t *testing.T) {
		materializer, err := NewMaterializer(&installerenv.Env{FIPSMode: true}, http.DefaultClient)
		require.NoError(t, err)
		assert.Equal(t, strings.Join([]string{materializationLayoutVersion, runtime.GOOS, runtime.GOARCH, "fips"}, "-"), materializer.MaterializationID())
	})
}

func TestMaterializerMaterialize(t *testing.T) {
	server := fixtures.NewServer(t)
	packageURL := server.PackageURL(fixtures.FixtureSimpleV1)
	digest := packageURL[strings.LastIndex(packageURL, "@sha256:")+len("@sha256:"):]
	materializer, err := NewMaterializer(&installerenv.Env{}, server.Client())
	require.NoError(t, err)
	destination := t.TempDir()

	err = materializer.Materialize(context.Background(), authoredscripts.Descriptor{
		Package: fixtures.FixtureSimpleV1.Package,
		Version: fixtures.FixtureSimpleV1.Version,
		URL:     packageURL,
		SHA256:  digest,
	}, destination)

	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "executable.sh"))
	require.NoError(t, err)
}

func TestMaterializerRejectsPackageMetadataBeforeExtraction(t *testing.T) {
	server := fixtures.NewServer(t)
	packageURL := server.PackageURL(fixtures.FixtureSimpleV1)
	digest := packageURL[strings.LastIndex(packageURL, "@sha256:")+len("@sha256:"):]
	materializer, err := NewMaterializer(&installerenv.Env{}, server.Client())
	require.NoError(t, err)
	destination := t.TempDir()

	err = materializer.Materialize(context.Background(), authoredscripts.Descriptor{
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
