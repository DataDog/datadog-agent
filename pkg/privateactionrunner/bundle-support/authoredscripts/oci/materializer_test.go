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
	"strconv"
	"strings"
	"testing"

	ociimage "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
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
	artifact := authoredscripts.LocalArtifact{Directory: destination}
	_, err = os.Stat(filepath.Join(artifact.ScriptDirectory(), "executable.sh"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "executable.sh"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestMaterializerMaterializeWithExtension(t *testing.T) {
	server := fixtures.NewServer(t)
	packageURL := server.PackageURL(fixtures.FixtureSimpleV1WithExtension)
	digest := packageURL[strings.LastIndex(packageURL, "@sha256:")+len("@sha256:"):]
	materializer, err := NewMaterializer(&installerenv.Env{}, server.Client())
	require.NoError(t, err)
	destination := t.TempDir()

	err = materializer.Materialize(context.Background(), authoredscripts.Descriptor{
		Package: fixtures.FixtureSimpleV1WithExtension.Package,
		Version: fixtures.FixtureSimpleV1WithExtension.Version,
		URL:     packageURL,
		SHA256:  digest,
	}, destination)

	require.NoError(t, err)
	artifact := authoredscripts.LocalArtifact{Directory: destination}
	_, err = os.Stat(filepath.Join(artifact.ScriptDirectory(), "executable.sh"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(artifact.DependencyDirectory("simple-extension"), "extension.sh"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(artifact.ScriptDirectory(), "extension.sh"))
	require.ErrorIs(t, err, os.ErrNotExist)
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

func TestMaterializerPackageSizeLimit(t *testing.T) {
	const twoGiB = uint64(2 << 30)
	for _, test := range []struct {
		name    string
		size    uint64
		wantErr string
	}{
		{name: "zero size", wantErr: "missing a nonzero size annotation"},
		{name: "over limit", size: twoGiB + 1, wantErr: "exceeds the"},
		{name: "at limit", size: twoGiB},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := fixtures.NewServer(t)
			packageURL := packageURLWithSize(t, server, test.size)
			materializer, err := NewMaterializer(&installerenv.Env{}, http.DefaultClient)
			require.NoError(t, err)
			destination := t.TempDir()

			err = materializer.Materialize(context.Background(), authoredscripts.Descriptor{
				Package: fixtures.FixtureSimpleV1.Package,
				Version: fixtures.FixtureSimpleV1.Version,
				URL:     packageURL,
			}, destination)

			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				entries, readErr := os.ReadDir(destination)
				require.NoError(t, readErr)
				assert.Empty(t, entries)
				return
			}
			require.NoError(t, err)
			artifact := authoredscripts.LocalArtifact{Directory: destination}
			_, err = os.Stat(filepath.Join(artifact.ScriptDirectory(), "executable.sh"))
			require.NoError(t, err)
		})
	}
}

func packageURLWithSize(t *testing.T, server *fixtures.Server, size uint64) string {
	t.Helper()

	downloader := fleetoci.NewDownloader(&installerenv.Env{}, http.DefaultClient)
	fixture, err := downloader.Download(context.Background(), server.PackageLayoutURL(fixtures.FixtureSimpleV1))
	require.NoError(t, err)
	image := mutate.Annotations(fixture.Image, map[string]string{fleetoci.AnnotationSize: strconv.FormatUint(size, 10)}).(ociimage.Image)
	path, err := layout.Write(t.TempDir(), empty.Index)
	require.NoError(t, err)
	require.NoError(t, path.AppendImage(image, layout.WithPlatform(ociimage.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH})))
	return "file://" + string(path)
}
