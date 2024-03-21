// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the updater is not supported on windows
//go:build !windows

package updater

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	pkg         string
	version     string
	layoutPath  string
	contentPath string
	configPath  string
	indexDigest string
}

var (
	fixtureSimpleV1 = fixture{
		pkg:         "simple",
		version:     "v1",
		layoutPath:  "fixtures/oci-layout-simple-v1.tar",
		contentPath: "fixtures/simple-v1",
		configPath:  "fixtures/simple-v1-config",
	}
	fixtureSimpleV2 = fixture{
		pkg:         "simple",
		version:     "v2",
		layoutPath:  "fixtures/oci-layout-simple-v2.tar",
		contentPath: "fixtures/simple-v2",
		configPath:  "fixtures/simple-v2-config",
	}
	fixtureSimpleV1Linux2Amd128 = fixture{
		pkg:         "simple",
		version:     "v1",
		layoutPath:  "fixtures/oci-layout-simple-v1-linux2-amd128.tar",
		contentPath: "fixtures/simple-v1",
	}
	ociFixtures = []*fixture{&fixtureSimpleV1, &fixtureSimpleV2, &fixtureSimpleV1Linux2Amd128}
)

//go:embed fixtures/*
var fixturesFS embed.FS

func buildOCIRegistry(t *testing.T) *httptest.Server {
	s := httptest.NewServer(registry.New())
	for _, f := range ociFixtures {
		tmpDir := t.TempDir()
		file, err := fixturesFS.Open(f.layoutPath)
		require.NoError(t, err)
		err = extractTarArchive(file, tmpDir, 1<<30)
		require.NoError(t, err)

		layout, err := layout.FromPath(tmpDir)
		require.NoError(t, err)
		index, err := layout.ImageIndex()
		require.NoError(t, err)

		url, err := url.Parse(s.URL)
		require.NoError(t, err)
		src := path.Join(url.Host, f.pkg)
		ref, err := name.ParseReference(src)
		require.NoError(t, err)
		err = remote.WriteIndex(ref, index)
		require.NoError(t, err)

		digest, err := index.Digest()
		require.NoError(t, err)
		f.indexDigest = digest.String()
	}
	return s
}

type testFixturesServer struct {
	t    *testing.T
	s    *httptest.Server
	soci *httptest.Server
}

func newTestFixturesServer(t *testing.T) *testFixturesServer {
	return &testFixturesServer{
		t:    t,
		s:    httptest.NewServer(http.FileServer(http.FS(fixturesFS))),
		soci: buildOCIRegistry(t),
	}
}

func (s *testFixturesServer) Downloader() *downloader {
	return newDownloader(s.s.Client(), "")
}

func (s *testFixturesServer) DownloaderOCI() *downloader {
	return newDownloader(s.soci.Client(), "")
}

func (s *testFixturesServer) DownloaderOCIRegistryOverride() *downloader {
	return newDownloader(s.soci.Client(), "my.super/registry")
}

func (s *testFixturesServer) Package(f fixture) Package {
	file, err := fixturesFS.Open(f.layoutPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, file)
	if err != nil {
		panic(err)
	}
	return Package{
		Name:    f.pkg,
		Version: f.version,
		URL:     s.s.URL + "/" + f.layoutPath,
		Size:    n,
		SHA256:  fmt.Sprintf("%x", hash.Sum(nil)),
	}
}

func (s *testFixturesServer) PackageOCI(f fixture) Package {
	return Package{
		Name:    f.pkg,
		Version: f.version,
		URL:     fmt.Sprintf("oci://%s/%s@%s", strings.TrimPrefix(s.soci.URL, "http://"), f.pkg, f.indexDigest),
	}
}

func (s *testFixturesServer) PackageFS(f fixture) fs.FS {
	fs, err := fs.Sub(fixturesFS, f.contentPath)
	if err != nil {
		panic(err)
	}
	return fs
}

func (s *testFixturesServer) ConfigFS(f fixture) fs.FS {
	if f.configPath == "" {
		return os.DirFS(s.t.TempDir())
	}
	fs, err := fs.Sub(fixturesFS, f.configPath)
	if err != nil {
		panic(err)
	}
	return fs
}

func (s *testFixturesServer) Image(f fixture) oci.Image {
	tmpDir := s.t.TempDir()
	image, err := s.Downloader().Download(context.Background(), tmpDir, s.Package(f))
	if err != nil {
		panic(err)
	}
	return image
}

func (s *testFixturesServer) Catalog() catalog {
	return catalog{
		Packages: []Package{
			s.Package(fixtureSimpleV1),
			s.Package(fixtureSimpleV2),
		},
	}
}

func (s *testFixturesServer) Close() {
	s.s.Close()
	s.soci.Close()
}

func TestDownload(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.Downloader()

	image, err := d.Download(context.Background(), t.TempDir(), s.Package(fixtureSimpleV1))
	assert.NoError(t, err)
	tmpDir := t.TempDir()
	err = extractPackageLayers(image, t.TempDir(), tmpDir)
	assert.NoError(t, err)
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), os.DirFS(tmpDir))
}

func TestDownloadInvalidHash(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.Downloader()

	pkg := s.Package(fixtureSimpleV1)
	pkg.SHA256 = "2857b8e9faf502169c9cfaf6d4ccf3a035eccddc0f5b87c613b673a807ff6d23"
	_, err := d.Download(context.Background(), t.TempDir(), pkg)
	assert.Error(t, err)
}

func TestDownloadPlatformNotAvailable(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.Downloader()

	pkg := s.Package(fixtureSimpleV1Linux2Amd128)
	_, err := d.Download(context.Background(), t.TempDir(), pkg)
	assert.Error(t, err)
}

func TestDownloadRegistry(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.DownloaderOCI()

	image, err := d.Download(context.Background(), t.TempDir(), s.PackageOCI(fixtureSimpleV1))
	assert.NoError(t, err)
	tmpDir := t.TempDir()
	err = extractPackageLayers(image, t.TempDir(), tmpDir)
	assert.NoError(t, err)
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), os.DirFS(tmpDir))
}

func TestDownloadRegistryWithOverride(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.DownloaderOCIRegistryOverride()

	_, err := d.Download(context.Background(), t.TempDir(), s.PackageOCI(fixtureSimpleV1))
	assert.Error(t, err) // Host not found
}

func TestGetRegistryURL(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()

	pkg := Package{
		Name:    "simple",
		Version: "v1",
		URL:     s.soci.URL + "/simple@sha256:2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d",
		SHA256:  "2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d",
	}

	d := s.DownloaderOCI()
	url := d.getRegistryURL(pkg)
	assert.Equal(t, s.soci.URL+"/simple@sha256:2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d", url)

	d = s.DownloaderOCIRegistryOverride()
	url = d.getRegistryURL(pkg)
	assert.Equal(t, "my.super/registry/simple@sha256:2aaf415ad1bd66fd9ba5214603c7fb27ef2eb595baf21222cde22846e02aab4d", url)
}

func TestDownloadOCIPlatformNotAvailable(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.DownloaderOCI()

	pkg := s.PackageOCI(fixtureSimpleV1Linux2Amd128)
	_, err := d.Download(context.Background(), t.TempDir(), pkg)
	assert.Error(t, err)
}
