// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package oci

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/fixtures"
	"github.com/google/go-containerregistry/pkg/authn"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/google"
)

type testDownloadServer struct {
	*fixtures.Server
}

func newTestDownloadServer(t *testing.T) *testDownloadServer {
	return &testDownloadServer{
		Server: fixtures.NewServer(t),
	}
}

func (s *testDownloadServer) Downloader() *Downloader {
	return NewDownloader(&env.Env{}, s.Client())
}

func (s *testDownloadServer) DownloaderWithEnv(env *env.Env) *Downloader {
	return NewDownloader(env, s.Client())
}

func (s *testDownloadServer) Image(f fixtures.Fixture) oci.Image {
	downloadedPackage, err := s.Downloader().Download(context.Background(), s.PackageURL(f))
	if err != nil {
		panic(err)
	}
	return downloadedPackage.Image
}

func TestDownload(t *testing.T) {
	s := newTestDownloadServer(t)
	d := s.Downloader()

	downloadedPackage, err := d.Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Package, downloadedPackage.Name)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, downloadedPackage.Version)
	assert.NotZero(t, downloadedPackage.Size)
	tmpDir := t.TempDir()
	err = downloadedPackage.ExtractLayers(DatadogPackageLayerMediaType, tmpDir)
	assert.NoError(t, err)
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), os.DirFS(tmpDir))
}

func TestDownloadLayout(t *testing.T) {
	s := newTestDownloadServer(t)
	d := s.Downloader()

	downloadedPackage, err := d.Download(context.Background(), s.PackageLayoutURL(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Package, downloadedPackage.Name)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, downloadedPackage.Version)
	assert.NotZero(t, downloadedPackage.Size)
	tmpDir := t.TempDir()
	err = downloadedPackage.ExtractLayers(DatadogPackageLayerMediaType, tmpDir)
	assert.NoError(t, err)
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), os.DirFS(tmpDir))
}

func TestDownloadInvalidHash(t *testing.T) {
	s := newTestDownloadServer(t)
	d := s.Downloader()

	pkgURL := s.PackageURL(fixtures.FixtureSimpleV1)
	pkgURL = pkgURL[:strings.Index(pkgURL, "@sha256:")] + "@sha256:2857b8e9faf502169c9cfaf6d4ccf3a035eccddc0f5b87c613b673a807ff6d23"
	_, err := d.Download(context.Background(), pkgURL)
	assert.Error(t, err)
}

func TestDownloadPlatformNotAvailable(t *testing.T) {
	s := newTestDownloadServer(t)
	d := s.Downloader()

	pkg := s.PackageURL(fixtures.FixtureSimpleV1Linux2Amd128)
	_, err := d.Download(context.Background(), pkg)
	assert.Error(t, err)
}

func TestDownloadRegistryWithOverride(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	d := s.DownloaderWithEnv(&env.Env{
		RegistryOverride: "fake.io",
	})

	_, err := d.Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1))
	assert.Error(t, err) // Host not found
}

func TestGetRefAndKeychain(t *testing.T) {
	type test struct {
		registryOverride       string
		regOverrideByImage     map[string]string
		registryAuthOverride   string
		regAuthOverrideByImage map[string]string
		url                    string
		expectedRef            string
		expectedKeychain       authn.Keychain
	}

	tests := []test{
		{url: "docker.io/datadog/agent-package-dev:latest", expectedRef: "docker.io/datadog/agent-package-dev:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "gcr.io/datadoghq/agent-package@sha256:1234", expectedRef: "gcr.io/datadoghq/agent-package@sha256:1234", expectedKeychain: authn.DefaultKeychain},
		{url: "docker.io/datadog/agent-package-dev:latest", registryOverride: "fake.io", expectedRef: "fake.io/agent-package-dev:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "gcr.io/datadoghq/agent-package@sha256:1234", registryOverride: "fake.io", expectedRef: "fake.io/agent-package@sha256:1234", expectedKeychain: authn.DefaultKeychain},
		{
			url:                "docker.io/datadog/agent-package-dev:latest",
			regOverrideByImage: map[string]string{"agent-package-dev": "fake.io"},
			expectedRef:        "fake.io/agent-package-dev:latest",
			expectedKeychain:   authn.DefaultKeychain,
		},
		{
			url:                "gcr.io/datadoghq/agent-package@sha256:1234",
			regOverrideByImage: map[string]string{"agent-package": "fake.io"},
			expectedRef:        "fake.io/agent-package@sha256:1234",
			expectedKeychain:   authn.DefaultKeychain,
		},
		{
			url:                "gcr.io/datadoghq/agent-package@sha256:1234",
			regOverrideByImage: map[string]string{"agent-package": "fake.io"},
			expectedRef:        "fake.io/agent-package@sha256:1234",
			expectedKeychain:   authn.DefaultKeychain,
		},
		{
			url:                "gcr.io/datadoghq/agent-package@sha256:1234",
			registryOverride:   "fake-other.io",
			regOverrideByImage: map[string]string{"agent-package": "fake.io"},
			expectedRef:        "fake.io/agent-package@sha256:1234",
			expectedKeychain:   authn.DefaultKeychain,
		},
		{
			url:                "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			registryOverride:   "fake-other.io",
			regOverrideByImage: map[string]string{"agent-package": "fake.io"},
			expectedRef:        "fake-other.io/agent-package-dev@sha256:1234",
			expectedKeychain:   authn.DefaultKeychain,
		},
		{
			url:                  "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			registryAuthOverride: "gcr",
			expectedRef:          "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			expectedKeychain:     google.Keychain,
		},
		{
			url:                    "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			regAuthOverrideByImage: map[string]string{"agent-package": "gcr"},
			expectedRef:            "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			expectedKeychain:       authn.DefaultKeychain,
		},
		{
			url:                    "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			regAuthOverrideByImage: map[string]string{"agent-package-dev": "gcr"},
			expectedRef:            "gcr.io/datadoghq/agent-package-dev@sha256:1234",
			expectedKeychain:       google.Keychain,
		},
	}

	for _, tt := range tests {
		env := &env.Env{
			RegistryOverride:            tt.registryOverride,
			RegistryOverrideByImage:     tt.regOverrideByImage,
			RegistryAuthOverride:        tt.registryAuthOverride,
			RegistryAuthOverrideByImage: tt.regAuthOverrideByImage,
		}
		actual := getRefAndKeychain(env, tt.url)
		assert.Equal(t, tt.expectedRef, actual.ref)
		assert.Equal(t, tt.expectedKeychain, actual.keychain)
	}
}

func TestPackageURL(t *testing.T) {
	type test struct {
		site     string
		pkg      string
		version  string
		expected string
	}

	tests := []test{
		{site: "datad0g.com", pkg: "datadog-agent", version: "latest", expected: "oci://docker.io/datadog/agent-package-dev:latest"},
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", expected: "oci://gcr.io/datadoghq/agent-package:1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.site, func(t *testing.T) {
			actual := PackageURL(&env.Env{Site: tt.site}, tt.pkg, tt.version)
			if actual != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}
