// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const testDepPackage = "datadog-agent-integration-deps"

func writeDepsManifest(t *testing.T, path string) {
	t.Helper()
	err := os.WriteFile(path, []byte(`{"checks": {"mysql": ["`+testDepPackage+`"], "gunicorn": ["`+testDepPackage+`"]}}`), 0o644)
	require.NoError(t, err)
}

// setDepsManifest points the loader at a fresh manifest and resets the
// manifest cache, so tests are independent of each other.
func setDepsManifest(t *testing.T) {
	t.Helper()
	depsManifestOnce = sync.Once{}
	depsManifestData = nil
	t.Cleanup(func() {
		depsManifestOnce = sync.Once{}
		depsManifestData = nil
	})
	cfg := configmock.New(t)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	writeDepsManifest(t, manifestPath)
	cfg.Set("on_demand_python_deps.manifest_path", manifestPath, pkgconfigmodel.SourceFile)
}

// setDepsPackagesRoot points the package probe at a temp packages root and
// restores the default afterwards.
func setDepsPackagesRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	prev := depsPackagesRoot
	depsPackagesRoot = root
	t.Cleanup(func() { depsPackagesRoot = prev })
	return root
}

func installTestDepPackage(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, testDepPackage, "stable", "site-packages"), 0o755))
}

func TestDepsPendingFor(t *testing.T) {
	setDepsManifest(t)
	root := setDepsPackagesRoot(t)

	assert.Equal(t, []string{testDepPackage}, depsPendingFor("mysql"),
		"the dependency package is not installed, mysql should be deferred")
	assert.Equal(t, []string{testDepPackage}, depsPendingFor("gunicorn"))

	installTestDepPackage(t, root)
	assert.Empty(t, depsPendingFor("mysql"), "the package is installed, nothing should be missing")

	assert.Empty(t, depsPendingFor("disk"), "a check without declared packages is never deferred")
}

func TestDepsPackageImportPath(t *testing.T) {
	setDepsPackagesRoot(t)
	configmock.New(t)
	assert.Equal(t, filepath.Join(depsPackagesRoot, testDepPackage, "stable", "site-packages"), depsPackageImportPath())
}

func TestDepsPackageImportPathConfigured(t *testing.T) {
	setDepsPackagesRoot(t)
	cfg := configmock.New(t)
	cfg.Set("on_demand_python_deps.dep_package_name", "custom-deps", pkgconfigmodel.SourceFile)
	assert.Equal(t, filepath.Join(depsPackagesRoot, "custom-deps", "stable", "site-packages"), depsPackageImportPath())
}
