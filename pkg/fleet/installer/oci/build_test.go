// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDist(t *testing.T, sitePackages, rawName, metadata string, topLevelPaths ...string) {
	t.Helper()
	distDir := filepath.Join(sitePackages, rawName+".dist-info")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "METADATA"), []byte(metadata), 0o644))
	record := &strings.Builder{}
	for _, path := range topLevelPaths {
		switch {
		case strings.Contains(path, "/"):
			record.WriteString(path + "/__init__.py,,\n")
		case strings.HasSuffix(path, ".py"):
			record.WriteString(path + ",,\n")
		default:
			record.WriteString(path + "/__init__.py,,\n")
		}
	}
	record.WriteString(rawName + ".dist-info/METADATA,,\n")
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "RECORD"), []byte(record.String()), 0o644))
	for _, path := range topLevelPaths {
		switch {
		case strings.Contains(path, "/"):
			require.NoError(t, os.MkdirAll(filepath.Join(sitePackages, path), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(sitePackages, path, "__init__.py"), []byte("# "+path), 0o644))
		case strings.HasSuffix(path, ".py"):
			require.NoError(t, os.WriteFile(filepath.Join(sitePackages, path), []byte("# "+path), 0o644))
		default:
			require.NoError(t, os.MkdirAll(filepath.Join(sitePackages, path), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(sitePackages, path, "__init__.py"), []byte("# "+path), 0o644))
		}
	}
}

func testSitePackages(t *testing.T) *SitePackages {
	t.Helper()
	root := t.TempDir()
	sitePackages := filepath.Join(root, "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(sitePackages, 0o755))

	writeDist(t, sitePackages, "datadog_mysql-7.5.1",
		"Requires-Dist: datadog-checks-base>=37.0.0\nRequires-Dist: PyMySQL==1.1.0; extra == \"deps\"\n",
		"datadog_checks/mysql")
	writeDist(t, sitePackages, "datadog_gunicorn-4.5.0",
		"Requires-Dist: datadog-checks-base>=37.0.0\nRequires-Dist: psutil==6.0.0; extra == \"deps\"\n",
		"datadog_checks/gunicorn")
	writeDist(t, sitePackages, "datadog_checks_base-37.0.0",
		"Requires-Dist: requests>=2.0.0\nRequires-Dist: psutil==6.0.0; extra == \"deps\"\n",
		"datadog_checks/base")
	writeDist(t, sitePackages, "PyMySQL-1.1.0",
		"Requires-Dist: cryptography>=42.0.0\n",
		"pymysql")
	writeDist(t, sitePackages, "psutil-6.0.0",
		"Requires-Dist: \n",
		"psutil")
	writeDist(t, sitePackages, "cryptography-42.0.0",
		"Requires-Dist: \n",
		"cryptography")
	writeDist(t, sitePackages, "requests-2.32.0",
		"Requires-Dist: \n",
		"requests")

	site, err := ParseSitePackages(sitePackages)
	require.NoError(t, err)
	return site
}

func TestParseSitePackages(t *testing.T) {
	site := testSitePackages(t)
	assert.Equal(t, "python3.12", site.PythonDir)
	assert.Len(t, site.Dists, 7)
	assert.Contains(t, site.Dists, "datadog_mysql")
	assert.Equal(t, []string{"datadog_checks", "datadog_mysql-7.5.1.dist-info"}, site.Dists["datadog_mysql"].TopLevelPaths)
}

func TestDepsManifest(t *testing.T) {
	site := testSitePackages(t)
	manifest := site.DepsManifest("datadog-agent-integration-deps")
	assert.Equal(t, map[string][]string{
		"mysql":    {"datadog-agent-integration-deps"},
		"gunicorn": {"datadog-agent-integration-deps"},
	}, manifest)
}

func TestDepsPackageDists(t *testing.T) {
	site := testSitePackages(t)
	dists := site.DepsPackageDists()
	names := []string{}
	for _, dist := range dists {
		names = append(names, dist.Name)
	}
	// psutil is a base-check dependency through its own deps group, but it is
	// also required by gunicorn, so it must ship. requests only belongs to the
	// base closure and must stay in the agent package.
	assert.Equal(t, []string{"cryptography", "psutil", "pymysql"}, names)
}

func TestDepsPackageDistsIgnoresOtherExtras(t *testing.T) {
	root := t.TempDir()
	sitePackages := filepath.Join(root, "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(sitePackages, 0o755))

	writeDist(t, sitePackages, "datadog_gunicorn-4.5.0",
		"Requires-Dist: datadog-checks-base>=37.0.0\nRequires-Dist: psutil==6.0.0; extra == \"deps\"\n",
		"datadog_checks/gunicorn")
	writeDist(t, sitePackages, "datadog_checks_base-37.0.0",
		"Requires-Dist: requests>=2.0.0\n",
		"datadog_checks/base")
	writeDist(t, sitePackages, "psutil-6.0.0",
		"Requires-Dist: ipaddress; extra == \"test\"\n",
		"psutil")
	writeDist(t, sitePackages, "ipaddress-1.0.23",
		"Requires-Dist: \n",
		"ipaddress")
	writeDist(t, sitePackages, "requests-2.32.0",
		"Requires-Dist: \n",
		"requests")

	site, err := ParseSitePackages(sitePackages)
	require.NoError(t, err)
	dists := site.DepsPackageDists()
	names := []string{}
	for _, dist := range dists {
		names = append(names, dist.Name)
	}
	assert.Equal(t, []string{"psutil"}, names, "test-only extras must not ship in the deps package")
}

func TestBuildDepsPackageLayer(t *testing.T) {
	site := testSitePackages(t)
	dists := site.DepsPackageDists()
	tarBytes, err := site.BuildDepsPackageLayer(dists)
	require.NoError(t, err)

	entries := map[string]bool{}
	reader := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		entries[header.Name] = true
	}
	assert.True(t, entries["site-packages/pymysql/__init__.py"])
	assert.True(t, entries["site-packages/psutil/__init__.py"])
	assert.True(t, entries["site-packages/PyMySQL-1.1.0.dist-info/METADATA"])
	assert.False(t, entries["site-packages/requests/__init__.py"])
}

func TestBuildDepsPackageLayerIncludesTopLevelFiles(t *testing.T) {
	root := t.TempDir()
	sitePackages := filepath.Join(root, "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(sitePackages, 0o755))
	writeDist(t, sitePackages, "datadog_mysql-7.5.1",
		"Requires-Dist: six==1.16.0; extra == \"deps\"\n",
		"datadog_checks/mysql")
	writeDist(t, sitePackages, "six-1.16.0",
		"Requires-Dist: \n",
		"six.py")

	site, err := ParseSitePackages(sitePackages)
	require.NoError(t, err)
	tarBytes, err := site.BuildDepsPackageLayer(site.DepsPackageDists())
	require.NoError(t, err)

	found := false
	reader := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if header.Name == "site-packages/six.py" {
			found = true
		}
	}
	assert.True(t, found, "top-level .py files must be included in the package layer")
}

func TestBuildDepsPackage(t *testing.T) {
	site := testSitePackages(t)
	dists := site.DepsPackageDists()
	image, err := site.BuildDepsPackage("datadog-agent-integration-deps", "7.99.0-1", dists)
	require.NoError(t, err)

	manifest, err := image.Manifest()
	require.NoError(t, err)
	assert.Equal(t, "datadog-agent-integration-deps", manifest.Annotations[AnnotationPackage])
	assert.Equal(t, "7.99.0-1", manifest.Annotations[AnnotationVersion])
	require.Len(t, manifest.Layers, 1)
	assert.Equal(t, string(DatadogPackageLayerMediaType), string(manifest.Layers[0].MediaType))

	layers, err := image.Layers()
	require.NoError(t, err)
	rc, err := layers[0].Uncompressed()
	require.NoError(t, err)
	defer rc.Close()
	_, err = tar.NewReader(rc).Next()
	require.NoError(t, err, "layer must contain a readable tar")
}
