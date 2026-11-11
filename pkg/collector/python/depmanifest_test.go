// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func writeDepsManifest(t *testing.T, path string) {
	t.Helper()
	err := os.WriteFile(path, []byte(`{"checks": {"mysql": ["pymysql"], "gunicorn": ["psutil"]}}`), 0o644)
	require.NoError(t, err)
}

func TestDepsPendingFor(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("on_demand_python_deps.enabled", true, pkgconfigmodel.SourceFile)

	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	writeDepsManifest(t, manifestPath)
	cfg.Set("on_demand_python_deps.manifest_path", manifestPath, pkgconfigmodel.SourceFile)

	sitePackages := filepath.Join(t.TempDir(), "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(filepath.Join(sitePackages, "pymysql"), 0o755))

	prevPythonPath := PythonPath
	defer func() { PythonPath = prevPythonPath }()
	PythonPath = "['/usr/lib/python3.12/site-packages', '" + sitePackages + "']"

	missing := depsPendingFor("mysql")
	assert.Empty(t, missing, "pymysql is installed, mysql should not be deferred")

	missing = depsPendingFor("gunicorn")
	assert.Equal(t, []string{"psutil"}, missing, "psutil is missing, gunicorn should be deferred")

	missing = depsPendingFor("disk")
	assert.Empty(t, missing, "disk declares no dependency, nothing should be deferred")
}

func TestDepsPendingForDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("on_demand_python_deps.enabled", false, pkgconfigmodel.SourceFile)

	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	writeDepsManifest(t, manifestPath)
	cfg.Set("on_demand_python_deps.manifest_path", manifestPath, pkgconfigmodel.SourceFile)

	prevPythonPath := PythonPath
	defer func() { PythonPath = prevPythonPath }()
	PythonPath = "[]"
	assert.Empty(t, depsPendingFor("mysql"))
}

func TestDistInstalledMatchesNormalizedNames(t *testing.T) {
	sitePackages := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sitePackages, "PyMySQL-1.1.0.dist-info"), 0o755))

	assert.True(t, distInstalled(sitePackages, "pymysql"))
	assert.False(t, distInstalled(sitePackages, "psutil"))
}

func TestSitePackagesDirs(t *testing.T) {
	assert.Empty(t, sitePackagesDirs(), "empty PythonPath must yield no directories")

	prevPythonPath := PythonPath
	defer func() { PythonPath = prevPythonPath }()

	PythonPath = "/no/repr/here"
	assert.Empty(t, sitePackagesDirs())

	PythonPath = "['/opt/datadog-agent/embedded/lib/python3.12/site-packages', '/usr/lib/python3.12']"
	dirs := sitePackagesDirs()
	assert.Equal(t, []string{"/opt/datadog-agent/embedded/lib/python3.12/site-packages"}, dirs)
}
