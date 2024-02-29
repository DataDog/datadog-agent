// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"os"
	"testing"

	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
)

type fixture struct {
	pkg         string
	version     string
	layoutPath  string
	contentPath string
}

var (
	fixtureSimpleV1 = fixture{
		pkg:         "simple",
		version:     "v1",
		layoutPath:  "fixtures/oci-layout-simple-v1.tar",
		contentPath: "fixtures/simple-v1",
	}
	fixtureSimpleV2 = fixture{
		pkg:         "simple",
		version:     "v2",
		layoutPath:  "fixtures/oci-layout-simple-v2.tar",
		contentPath: "fixtures/simple-v2",
	}
)

//go:embed fixtures/*
var fixturesFS embed.FS

type testFixturesServer struct {
	t *testing.T
	s *httptest.Server
}

func newTestFixturesServer(t *testing.T) *testFixturesServer {
	return &testFixturesServer{
		t: t,
		s: httptest.NewServer(http.FileServer(http.FS(fixturesFS))),
	}
}

func (s *testFixturesServer) Downloader() *downloader {
	return newDownloader(s.s.Client())
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

func (s *testFixturesServer) PackageFS(f fixture) fs.FS {
	fs, err := fs.Sub(fixturesFS, f.contentPath)
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
}

func TestDownload(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	d := s.Downloader()

	image, err := d.Download(context.Background(), t.TempDir(), s.Package(fixtureSimpleV1))
	assert.NoError(t, err)
	tmpDir := t.TempDir()
	err = extractPackageLayers(image, tmpDir)
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
