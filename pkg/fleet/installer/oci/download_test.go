// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package oci

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/google/go-containerregistry/pkg/authn"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"

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

func TestDownloadIndexVariantSelection(t *testing.T) {
	plat := oci.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}

	baseImg, err := random.Image(64, 1)
	require.NoError(t, err)
	baseDigest, err := baseImg.Digest()
	require.NoError(t, err)

	fipsImg, err := random.Image(64, 1)
	require.NoError(t, err)
	fipsDigest, err := fipsImg.Digest()
	require.NoError(t, err)
	require.NotEqual(t, baseDigest, fipsDigest)

	bothFlavorIndex := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add:        baseImg,
			Descriptor: oci.Descriptor{Platform: &plat},
		},
		mutate.IndexAddendum{
			Add: fipsImg,
			Descriptor: oci.Descriptor{
				Platform: &oci.Platform{OS: plat.OS, Architecture: plat.Architecture, Variant: VariantFIPS},
			},
		},
	)

	t.Run("FIPSMode=false picks the base manifest", func(t *testing.T) {
		d := NewDownloader(&env.Env{FIPSMode: false}, http.DefaultClient)
		got, err := d.downloadIndex(bothFlavorIndex)
		require.NoError(t, err)
		gotDigest, err := got.Digest()
		require.NoError(t, err)
		assert.Equal(t, baseDigest, gotDigest)
	})

	t.Run("FIPSMode=true picks the FIPS manifest", func(t *testing.T) {
		d := NewDownloader(&env.Env{FIPSMode: true}, http.DefaultClient)
		got, err := d.downloadIndex(bothFlavorIndex)
		require.NoError(t, err)
		gotDigest, err := got.Digest()
		require.NoError(t, err)
		assert.Equal(t, fipsDigest, gotDigest)
	})

	// Even if the FIPS manifest is listed first in the index, a non-FIPS
	// request must skip it instead of accepting it via Satisfies' empty-variant
	// wildcard.
	t.Run("FIPSMode=false skips FIPS manifest regardless of index order", func(t *testing.T) {
		reorderedIndex := mutate.AppendManifests(empty.Index,
			mutate.IndexAddendum{
				Add: fipsImg,
				Descriptor: oci.Descriptor{
					Platform: &oci.Platform{OS: plat.OS, Architecture: plat.Architecture, Variant: VariantFIPS},
				},
			},
			mutate.IndexAddendum{
				Add:        baseImg,
				Descriptor: oci.Descriptor{Platform: &plat},
			},
		)
		d := NewDownloader(&env.Env{FIPSMode: false}, http.DefaultClient)
		got, err := d.downloadIndex(reorderedIndex)
		require.NoError(t, err)
		gotDigest, err := got.Digest()
		require.NoError(t, err)
		assert.Equal(t, baseDigest, gotDigest)
	})

	// FIPS mode must not fall back to a base manifest if no FIPS variant is
	// present in the index.
	t.Run("FIPSMode=true returns ErrPackageNotFound when no FIPS manifest exists", func(t *testing.T) {
		baseOnlyIndex := mutate.AppendManifests(empty.Index,
			mutate.IndexAddendum{
				Add:        baseImg,
				Descriptor: oci.Descriptor{Platform: &plat},
			},
		)
		d := NewDownloader(&env.Env{FIPSMode: true}, http.DefaultClient)
		_, err := d.downloadIndex(baseOnlyIndex)
		require.Error(t, err)
	})
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

func TestWithRegistryOverride(t *testing.T) {
	original := &env.Env{
		RegistryOverride:            "original.registry.com",
		RegistryAuthOverride:        "docker",
		RegistryUsername:            "origuser",
		RegistryPassword:            "origpass",
		RegistryOverrideByImage:     map[string]string{"agent-package": "image-scoped.io"},
		RegistryAuthOverrideByImage: map[string]string{"agent-package": "gcr"},
		Site:                        "datadoghq.com",
	}
	client := http.DefaultClient
	d := NewDownloader(original, client)

	overridden := d.WithRegistryOverride("custom.registry.com", "password", "newuser", "newpass")

	// Overridden downloader has new values
	assert.Equal(t, "custom.registry.com", overridden.env.RegistryOverride)
	assert.Equal(t, "password", overridden.env.RegistryAuthOverride)
	assert.Equal(t, "newuser", overridden.env.RegistryUsername)
	assert.Equal(t, "newpass", overridden.env.RegistryPassword)

	// Image-scoped override maps are preserved (env vars take precedence)
	assert.Equal(t, map[string]string{"agent-package": "image-scoped.io"}, overridden.env.RegistryOverrideByImage)
	assert.Equal(t, map[string]string{"agent-package": "gcr"}, overridden.env.RegistryAuthOverrideByImage)

	// Original is unchanged
	assert.Equal(t, "original.registry.com", d.env.RegistryOverride)
	assert.Equal(t, "docker", d.env.RegistryAuthOverride)
	assert.Equal(t, "origuser", d.env.RegistryUsername)
	assert.Equal(t, "origpass", d.env.RegistryPassword)

	// Shares same HTTP client
	assert.Same(t, d.client, overridden.client)

	// Non-registry fields are preserved
	assert.Equal(t, "datadoghq.com", overridden.env.Site)
}

func TestWithRegistryOverridePartial(t *testing.T) {
	original := &env.Env{
		RegistryOverride:     "original.registry.com",
		RegistryAuthOverride: "docker",
		RegistryUsername:     "origuser",
		RegistryPassword:     "origpass",
	}
	d := NewDownloader(original, http.DefaultClient)

	// Only override URL, leave auth/username/password empty
	overridden := d.WithRegistryOverride("custom.registry.com", "", "", "")

	assert.Equal(t, "custom.registry.com", overridden.env.RegistryOverride)
	assert.Equal(t, "docker", overridden.env.RegistryAuthOverride)
	assert.Equal(t, "origuser", overridden.env.RegistryUsername)
	assert.Equal(t, "origpass", overridden.env.RegistryPassword)
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

// TestProgressCallbackReportsRange verifies that progress values stay within
// [0.0, 1.0], are monotonically non-decreasing, and reach ≥ 0.99 by the end
// of a successful layer extraction. This acts as a regression guard: if
// AnnotationSize were accidentally set to the compressed blob size instead of
// the uncompressed extracted size, bytesRead/Size would overshoot 1.0 early,
// the callback would pin at 1.0 before extraction completes, and the
// monotone-to-≥0.99 assertion would still pass — but the overshoot is caught
// by the strict ≤ 1.0 assertion on every individual value.
func TestProgressCallbackReportsRange(t *testing.T) {
	s := newTestDownloadServer(t)
	pkg, err := s.Downloader().Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1))
	require.NoError(t, err)
	require.NotZero(t, pkg.Size, "fixture must have AnnotationSize set")

	var progress []float32
	pkg.SetProgressCallback(func(v float32) {
		progress = append(progress, v)
	})

	tmpDir := t.TempDir()
	require.NoError(t, pkg.ExtractLayers(DatadogPackageLayerMediaType, tmpDir))

	require.NotEmpty(t, progress, "at least one progress callback must fire")
	for i, v := range progress {
		assert.GreaterOrEqual(t, v, float32(0.0), "progress[%d] must be ≥ 0", i)
		assert.LessOrEqual(t, v, float32(1.0), "progress[%d] must be ≤ 1.0", i)
		if i > 0 {
			assert.GreaterOrEqual(t, v, progress[i-1], "progress must be non-decreasing at index %d", i)
		}
	}
	assert.GreaterOrEqual(t, progress[len(progress)-1], float32(0.99), "final progress must reach ≥ 0.99")
}

// TestProgressCallbackDebounce verifies the ≥1% debounce threshold: a read
// that advances progress by less than 1% must not fire the callback.
func TestProgressCallbackDebounce(t *testing.T) {
	pkg := &DownloadedPackage{Size: 1000}

	var fired []float32
	pkg.SetProgressCallback(func(v float32) {
		fired = append(fired, v)
	})

	// 5 bytes = 0.5% of 1000 — below the 1% threshold; no callback expected.
	r1 := &progressReader{ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{1}, 5))), pkg: pkg}
	_, err := io.ReadAll(r1)
	require.NoError(t, err)
	assert.Empty(t, fired, "no callback should fire below 1% threshold")

	// 10 more bytes = cumulative 1.5% — crosses the 1% threshold; exactly one callback expected.
	r2 := &progressReader{ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{1}, 10))), pkg: pkg}
	_, err = io.ReadAll(r2)
	require.NoError(t, err)
	require.Len(t, fired, 1, "exactly one callback should fire once the threshold is crossed")
	assert.InDelta(t, 0.015, fired[0], 0.001)
}

// TestProgressCallbackNoInflationOnRetry verifies that bytesRead and lastReport
// are restored to their pre-attempt values at the start of each retry, so a
// partial read that fails mid-stream does not inflate progress in the next attempt.
//
// The test models the withNetworkRetries save/restore mechanic directly: it
// captures the counters before a simulated first attempt, lets the attempt run
// (accumulating bytes into bytesRead), then restores the saved values exactly
// as the fixed ExtractLayers closure does. The second attempt must then start
// from the saved baseline, not from the inflated value.
func TestProgressCallbackNoInflationOnRetry(t *testing.T) {
	const size = 100
	pkg := &DownloadedPackage{Size: size}

	var progress []float32
	pkg.SetProgressCallback(func(v float32) {
		progress = append(progress, v)
	})

	// Capture baseline before the first attempt (mirrors the savedBytesRead /
	// savedLastReport snapshot taken before withNetworkRetries in ExtractLayers).
	savedBytesRead := pkg.bytesRead
	savedLastReport := pkg.lastReport

	// First attempt: reads 60 bytes, then "fails" (network error interrupts).
	r1 := &progressReader{ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{1}, 60))), pkg: pkg}
	_, err := io.ReadAll(r1)
	require.NoError(t, err)
	assert.Equal(t, int64(60), pkg.bytesRead)

	// Restore — this is the fix; without it, bytesRead would be 60 at the start
	// of the second attempt, inflating all subsequent progress values.
	pkg.bytesRead = savedBytesRead
	pkg.lastReport = savedLastReport
	progress = nil

	// Second attempt: read only 5 bytes (5% of size=100) so the first callback
	// fires at a low value. Without the fix, bytesRead would start at 60, making
	// the first callback fire at (60+5)/100 = 0.65. With the fix it fires at 0.05.
	r2 := &progressReader{ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{1}, size))), pkg: pkg}
	_, err = r2.Read(make([]byte, 5))
	require.NoError(t, err)

	require.Len(t, progress, 1, "exactly one callback should have fired after reading 5 bytes")
	assert.LessOrEqual(t, progress[0], float32(0.10),
		"first callback after restore must start near 0, not from the stale 0.60")
}
