// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// depsManifest maps a check name to the OCI dependency packages the check
// requires (the fat datadog-agent-integration-deps package for every
// dep-having check today, per-check packages later). It is generated at
// build time from the integration wheels metadata and shipped with the
// agent (see cmd/integration-deps-builder).
type depsManifest struct {
	Checks map[string][]string `json:"checks"`
}

var (
	depsManifestOnce sync.Once
	depsManifestData *depsManifest
)

const defaultDepPackageName = "datadog-agent-integration-deps"

// depsPackagesRoot is the fleet installer packages root, under which
// installed packages expose their stable tree. Var for tests.
var depsPackagesRoot = paths.PackagesPath

// depsPackagePath returns the site-packages path of a dependency package's
// stable tree. The layout is the installer's: packages root and stable
// tree come from it, and the content dir is paths.DepsPackageContentDir,
// shared with the package builder. This is the only place the path is
// assembled: both the interpreter import path (see depsPackageImportPath)
// and the presence probe (see depPackageInstalled) go through it.
func depsPackagePath(pkg string) string {
	return filepath.Join(depsPackagesRoot, pkg, "stable", paths.DepsPackageContentDir)
}

// loadDepsManifest reads the on-demand dependency manifest from the path
// configured in on_demand_python_deps.manifest_path. It returns nil when the
// manifest cannot be read, in which case dependency probing is skipped and
// checks load or fail exactly as before the feature.
func loadDepsManifest() *depsManifest {
	depsManifestOnce.Do(func() {
		cfg := pkgconfigsetup.Datadog()
		manifestPath := cfg.GetString("on_demand_python_deps.manifest_path")
		if manifestPath == "" {
			log.Debug("python loader: on_demand_python_deps.manifest_path is empty, dependency probing disabled")
			return
		}
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			log.Errorf("python loader: could not read dependency manifest %s: %v, dependency probing disabled", manifestPath, err)
			return
		}
		var m depsManifest
		if err := json.Unmarshal(data, &m); err != nil {
			log.Errorf("python loader: could not parse dependency manifest %s: %v, dependency probing disabled", manifestPath, err)
			return
		}
		depsManifestData = &m
	})
	return depsManifestData
}

// depsPendingFor returns the dependency packages declared for checkName
// that are not installed yet. It returns nil when nothing is declared, when
// probing is disabled, or on platforms without the on-demand install path
// (windows: the installer client is a stub), so callers fall back to normal
// load behavior.
func depsPendingFor(checkName string) []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	pkgs := declaredDeps(checkName)
	if len(pkgs) == 0 {
		return nil
	}
	missing := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		if !depPackageInstalled(pkg) {
			missing = append(missing, pkg)
		}
	}
	return missing
}

// declaredDeps returns the dependency packages declared for checkName in
// the dependency manifest, or nil when the manifest is not loaded.
func declaredDeps(checkName string) []string {
	m := loadDepsManifest()
	if m == nil {
		return nil
	}
	return m.Checks[checkName]
}

// depPackageInstalled reports whether the stable tree of the dependency
// package is present. The installer moves the whole package content into
// its versioned directory and only then points the stable link at it, so a
// resolving stable link implies the content is fully present. The
// interpreter has the package's stable site-packages on its import path
// (see depsPackageImportPath), so present also means importable.
func depPackageInstalled(pkg string) bool {
	_, err := os.Stat(depsPackagePath(pkg))
	return err == nil
}

// depsPackageImportPath returns the site-packages path of the configured
// dependency package, which is appended to the interpreter import path at
// boot. The path may dangle until the package is installed: CPython's
// FileFinder re-stats a path entry on each import attempt, so modules
// become importable as soon as the stable link points at an installed
// version. It returns "" on platforms without the on-demand install path.
func depsPackageImportPath() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	pkg := pkgconfigsetup.Datadog().GetString("on_demand_python_deps.dep_package_name")
	if pkg == "" {
		pkg = defaultDepPackageName
	}
	return depsPackagePath(pkg)
}
