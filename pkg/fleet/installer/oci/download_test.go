// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package oci

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"

	"github.com/google/go-containerregistry/pkg/authn"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/google"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

type testDownloadServer struct {
	*fixtures.Server
	m *testMirrorServer
}

func newTestDownloadServer(t *testing.T) *testDownloadServer {
	s := fixtures.NewServer(t)
	return &testDownloadServer{
		Server: s,
		m:      newTestMirrorServer(s.URL()),
	}
}

func (s *testDownloadServer) DownloaderWithMirror() *Downloader {
	return NewDownloader(&env.Env{Mirror: s.m.URL() + "/mirror"}, http.DefaultClient)
}

func (s *testDownloadServer) Downloader() *Downloader {
	return NewDownloader(&env.Env{}, http.DefaultClient)
}

func (s *testDownloadServer) DownloaderWithEnv(env *env.Env) *Downloader {
	return NewDownloader(env, http.DefaultClient)
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

func TestDownloadMirror(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	d := s.DownloaderWithMirror()

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

func TestDownloadConfigLayer(t *testing.T) {
	s := newTestDownloadServer(t)
	d := s.Downloader()

	downloadedPackage, err := d.Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Package, downloadedPackage.Name)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, downloadedPackage.Version)
	assert.NotZero(t, downloadedPackage.Size)
	tmpDir := t.TempDir()
	err = downloadedPackage.ExtractLayers(DatadogPackageExtensionLayerMediaType, tmpDir, LayerAnnotation{Key: "com.datadoghq.package.extension.name", Value: "simple-extension"})
	assert.NoError(t, err)

	extensionsFS := s.ExtensionsFS(fixtures.FixtureSimpleV1WithExtension)
	fixtures.AssertEqualFS(t, extensionsFS["simple-v1-extension"], os.DirFS(tmpDir))
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
		{url: "install.datad0g.com/agent-package:latest", expectedRef: "install.datad0g.com/agent-package:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "gcr.io/datadoghq/agent-package@sha256:1234", expectedRef: "gcr.io/datadoghq/agent-package@sha256:1234", expectedKeychain: authn.DefaultKeychain},
		{url: "install.datad0g.com/agent-package:latest", registryOverride: "fake.io", expectedRef: "fake.io/agent-package:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "install.datad0g.com/agent-package:latest", registryOverride: "http://fake.io", expectedRef: "fake.io/agent-package:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "install.datad0g.com/agent-package:latest", registryOverride: "https://fake.io", expectedRef: "fake.io/agent-package:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "install.datad0g.com/agent-package:latest", registryOverride: "https://fake.io:443", expectedRef: "fake.io:443/agent-package:latest", expectedKeychain: authn.DefaultKeychain},
		{url: "gcr.io/datadoghq/agent-package@sha256:1234", registryOverride: "fake.io", expectedRef: "fake.io/agent-package@sha256:1234", expectedKeychain: authn.DefaultKeychain},
		{
			url:                "install.datad0g.com/agent-package:latest",
			regOverrideByImage: map[string]string{"agent-package": "fake.io"},
			expectedRef:        "fake.io/agent-package:latest",
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
			url:                  "gcr.io/datadoghq/agent-package@sha256:1234",
			registryAuthOverride: "gcr",
			expectedRef:          "gcr.io/datadoghq/agent-package@sha256:1234",
			expectedKeychain:     google.Keychain,
		},
		{
			url:                    "gcr.io/datadoghq/agent-package@sha256:1234",
			regAuthOverrideByImage: map[string]string{"agent-package": "gcr"},
			expectedRef:            "gcr.io/datadoghq/agent-package@sha256:1234",
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
		{site: "datad0g.com", pkg: "datadog-agent", version: "latest", expected: "oci://install.datad0g.com/agent-package:latest"},
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", expected: "oci://install.datadoghq.com/agent-package:1.2.3"},
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

func TestIsStreamResetError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non stream reset error",
			err:      assert.AnError,
			expected: false,
		},
		{
			name:     "stream error - other error",
			err:      http2.StreamError{Code: http2.ErrCodeStreamClosed},
			expected: false,
		},
		{
			name:     "stream error - internal error - value",
			err:      http2.StreamError{Code: http2.ErrCodeInternal},
			expected: true,
		},
		{
			name:     "stream error - internal error - pointer",
			err:      &http2.StreamError{Code: http2.ErrCodeInternal},
			expected: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isStreamResetError(tc.err))
		})
	}
}

func TestGetRefAndKeychains(t *testing.T) {
	type test struct {
		name                    string
		url                     string
		registryOverride        string
		registryAuthOverride    string
		expectedRefAndKeychains []urlWithKeychain
		isProd                  bool
	}

	tests := []test{
		{
			name: "no override - staging",
			url:  "install.datad0g.com/agent-package:latest",
			expectedRefAndKeychains: []urlWithKeychain{
				{ref: "install.datad0g.com/agent-package:latest", keychain: authn.DefaultKeychain},
			},
		},
		{
			name:   "no override - prod",
			url:    "install.datadoghq.com/agent-package@sha256:1234",
			isProd: true,
			expectedRefAndKeychains: []urlWithKeychain{
				{ref: "install.datadoghq.com/agent-package@sha256:1234", keychain: authn.DefaultKeychain},
				{ref: "gcr.io/datadoghq/agent-package@sha256:1234", keychain: authn.DefaultKeychain},
			},
		},
		{
			name:   "no override - different url",
			url:    "mysuperregistry.tv/agent-package@sha256:1234",
			isProd: true,
			expectedRefAndKeychains: []urlWithKeychain{
				{ref: "mysuperregistry.tv/agent-package@sha256:1234", keychain: authn.DefaultKeychain},
				{ref: "install.datadoghq.com/agent-package@sha256:1234", keychain: authn.DefaultKeychain},
				{ref: "gcr.io/datadoghq/agent-package@sha256:1234", keychain: authn.DefaultKeychain},
			},
		},
		{
			name:                 "override",
			url:                  "gcr.io/datadoghq/agent-package@sha256:1234",
			registryOverride:     "mysuperregistry.tv",
			registryAuthOverride: "gcr",
			isProd:               true,
			expectedRefAndKeychains: []urlWithKeychain{
				{ref: "mysuperregistry.tv/agent-package@sha256:1234", keychain: google.Keychain},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := &env.Env{
				Site:                 "datad0g.com",
				RegistryOverride:     tt.registryOverride,
				RegistryAuthOverride: tt.registryAuthOverride,
			}
			if tt.isProd {
				env.Site = "datadoghq.com"
			}
			actual := getRefAndKeychains(env, tt.url)
			assert.Len(t, actual, len(tt.expectedRefAndKeychains))
			for i, a := range actual {
				assert.Equal(t, tt.expectedRefAndKeychains[i].ref, a.ref)
				assert.Equal(t, tt.expectedRefAndKeychains[i].keychain, a.keychain)
			}
		})
	}
}
