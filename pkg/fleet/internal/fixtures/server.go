// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fixtures contains test datadog package fixtures.
package fixtures

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/tar"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

// Fixture represents a package fixture.
type Fixture struct {
	Package     string
	Version     string
	layoutPath  string
	contentPath string
	configPath  string
	indexDigest string
}

var (
	// FixtureSimpleV1 is a simple package fixture with version v1.
	FixtureSimpleV1 = Fixture{
		Package:     "simple",
		Version:     "v1",
		layoutPath:  "oci-layout-simple-v1.tar",
		contentPath: "simple-v1",
		configPath:  "simple-v1-config",
	}
	// FixtureSimpleV2 is a simple package fixture with version v2.
	FixtureSimpleV2 = Fixture{
		Package:     "simple",
		Version:     "v2",
		layoutPath:  "oci-layout-simple-v2.tar",
		contentPath: "simple-v2",
		configPath:  "simple-v2-config",
	}
	// FixtureSimpleV1Linux2Amd128 is a simple package fixture with version v1 for linux/amd128.
	FixtureSimpleV1Linux2Amd128 = Fixture{
		Package:     "simple",
		Version:     "v1",
		layoutPath:  "oci-layout-simple-v1-linux2-amd128.tar",
		contentPath: "simple-v1",
	}
	ociFixtures = []*Fixture{&FixtureSimpleV1, &FixtureSimpleV2, &FixtureSimpleV1Linux2Amd128}
)

//go:embed *
var fixturesFS embed.FS

func extractLayoutsAndBuildRegistry(t *testing.T, layoutsDir string) *httptest.Server {
	s := httptest.NewServer(registry.New())
	for _, f := range ociFixtures {
		layoutDir := path.Join(layoutsDir, f.layoutPath)
		file, err := fixturesFS.Open(f.layoutPath)
		require.NoError(t, err)
		err = tar.Extract(file, layoutDir, 1<<30)
		require.NoError(t, err)

		layout, err := layout.FromPath(layoutDir)
		require.NoError(t, err)
		index, err := layout.ImageIndex()
		require.NoError(t, err)

		url, err := url.Parse(s.URL)
		require.NoError(t, err)
		src := path.Join(url.Host, f.Package)
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

// Server represents a test fixtures server.
type Server struct {
	t *testing.T
	s *httptest.Server

	layoutsDir string
}

// NewServer creates a new test fixtures server.
func NewServer(t *testing.T) *Server {
	layoutDir := t.TempDir()
	s := &Server{
		t:          t,
		s:          extractLayoutsAndBuildRegistry(t, layoutDir),
		layoutsDir: layoutDir,
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// PackageURL returns the package URL for the given fixture.
func (s *Server) PackageURL(f Fixture) string {
	return fmt.Sprintf("oci://%s/%s@%s", strings.TrimPrefix(s.s.URL, "http://"), f.Package, f.indexDigest)
}

// PackageLayoutURL returns the package layout URL for the given fixture.
func (s *Server) PackageLayoutURL(f Fixture) string {
	return fmt.Sprintf("file://%s/%s", s.layoutsDir, f.layoutPath)
}

// PackageFS returns the package FS for the given fixture.
func (s *Server) PackageFS(f Fixture) fs.FS {
	fs, err := fs.Sub(fixturesFS, f.contentPath)
	if err != nil {
		panic(err)
	}
	return fs
}

// ConfigFS returns the config FS for the given fixture.
func (s *Server) ConfigFS(f Fixture) fs.FS {
	if f.configPath == "" {
		return os.DirFS(s.t.TempDir())
	}
	fs, err := fs.Sub(fixturesFS, f.configPath)
	if err != nil {
		panic(err)
	}
	return fs
}

// Client returns the server client.
func (s *Server) Client() *http.Client {
	return s.s.Client()
}

// URL returns the server URL.
func (s *Server) URL() string {
	return s.s.URL
}

// Close closes the test fixtures server.
func (s *Server) Close() {
	s.s.Close()
}
