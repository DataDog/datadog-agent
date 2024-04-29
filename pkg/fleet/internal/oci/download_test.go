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

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/fixtures"
	oci "github.com/google/go-containerregistry/pkg/v1"
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
	return NewDownloader(s.Client(), "", RegistryAuthDefault)
}

func (s *testDownloadServer) DownloaderRegistryOverride() *Downloader {
	return NewDownloader(s.Client(), "my.super/registry", RegistryAuthDefault)
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
	defer s.Close()
	d := s.Downloader()

	downloadedPackage, err := d.Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Package, downloadedPackage.Name)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, downloadedPackage.Version)
	tmpDir := t.TempDir()
	err = downloadedPackage.ExtractLayers(DatadogPackageLayerMediaType, tmpDir)
	assert.NoError(t, err)
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), os.DirFS(tmpDir))
}

func TestDownloadLayout(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	d := s.Downloader()

	downloadedPackage, err := d.Download(context.Background(), s.PackageLayoutURL(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Package, downloadedPackage.Name)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, downloadedPackage.Version)
	tmpDir := t.TempDir()
	err = downloadedPackage.ExtractLayers(DatadogPackageLayerMediaType, tmpDir)
	assert.NoError(t, err)
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), os.DirFS(tmpDir))
}

func TestDownloadInvalidHash(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	d := s.Downloader()

	pkgURL := s.PackageURL(fixtures.FixtureSimpleV1)
	pkgURL = pkgURL[:strings.Index(pkgURL, "@sha256:")] + "@sha256:2857b8e9faf502169c9cfaf6d4ccf3a035eccddc0f5b87c613b673a807ff6d23"
	_, err := d.Download(context.Background(), pkgURL)
	assert.Error(t, err)
}

func TestDownloadPlatformNotAvailable(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	d := s.Downloader()

	pkg := s.PackageURL(fixtures.FixtureSimpleV1Linux2Amd128)
	_, err := d.Download(context.Background(), pkg)
	assert.Error(t, err)
}

func TestDownloadRegistryWithOverride(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	d := s.DownloaderRegistryOverride()

	_, err := d.Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1))
	assert.Error(t, err) // Host not found
}

func TestGetRegistryURL(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()

	testURL := s.URL() + "/simple@sha256:2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d"

	d := s.Downloader()
	url := d.getRegistryURL(testURL)
	assert.Equal(t, s.URL()+"/simple@sha256:2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d", url)

	d = s.DownloaderRegistryOverride()
	url = d.getRegistryURL(testURL)
	assert.Equal(t, "my.super/registry/simple@sha256:2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d", url)
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
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", expected: "oci://public.ecr.aws/datadog/agent-package:1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.site, func(t *testing.T) {
			actual := PackageURL(tt.site, tt.pkg, tt.version)
			if actual != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}
