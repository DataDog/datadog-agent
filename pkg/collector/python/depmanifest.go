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
	"strings"
	"sync"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// depsManifest maps a check name to the python import names of the
// third-party dependencies the check requires. It is generated at build time
// from the integration wheels metadata and shipped with the agent (see
// cmd/integration-deps-builder).
type depsManifest struct {
	Checks map[string][]string `json:"checks"`
}

var (
	depsManifestOnce sync.Once
	depsManifestData *depsManifest
)

// loadDepsManifest reads the on-demand dependency manifest from the path
// configured in on_demand_python_deps.manifest_path. It returns nil when the
// feature is disabled or when the manifest cannot be read, in which case
// dependency probing is skipped and checks load or fail exactly as they do
// today.
func loadDepsManifest() *depsManifest {
	depsManifestOnce.Do(func() {
		cfg := pkgconfigsetup.Datadog()
		if !cfg.GetBool("on_demand_python_deps.enabled") {
			return
		}
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

// depsPendingFor returns the python import names of the dependencies declared
// for checkName that are not importable yet. It returns nil when nothing is
// declared, when probing is disabled, or when the probe cannot run (no
// interpreter yet), so callers fall back to normal load behavior.
func depsPendingFor(checkName string) []string {
	deps := declaredDeps(checkName)
	if len(deps) == 0 {
		return nil
	}
	dirs := sitePackagesDirs()
	if len(dirs) == 0 {
		return nil
	}
	missing := make([]string, 0, len(deps))
	for _, dep := range deps {
		if !depImportable(dirs, dep) {
			missing = append(missing, dep)
		}
	}
	return missing
}

// declaredDeps returns the dependencies declared for checkName in the
// dependency manifest, or nil when the manifest is not loaded.
func declaredDeps(checkName string) []string {
	m := loadDepsManifest()
	if m == nil {
		return nil
	}
	return m.Checks[checkName]
}

// sitePackagesDirs parses PythonPath, the repr of sys.path, and returns the
// entries pointing to a site-packages directory. PythonPath is only set once
// the interpreter has been initialized, an empty value means the caller cannot
// probe yet.
func sitePackagesDirs() []string {
	if PythonPath == "" {
		return nil
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(PythonPath, "["), "]")
	if trimmed == PythonPath {
		return nil
	}
	dirs := []string{}
	for _, entry := range strings.Split(trimmed, ", ") {
		entry = strings.TrimSpace(entry)
		entry = strings.Trim(entry, "'\"")
		if strings.HasSuffix(entry, "site-packages") {
			dirs = append(dirs, entry)
		}
	}
	return dirs
}

// depImportable reports whether a python module named dep can be imported
// from one of the given site-packages directories. A dependency is
// considered importable when its module package or file exists, or when any
// installed distribution matches the import name.
func depImportable(dirs []string, dep string) bool {
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, dep)); err == nil {
			return true
		}
		if _, err := os.Stat(filepath.Join(dir, dep+".py")); err == nil {
			return true
		}
		if distInstalled(dir, dep) {
			return true
		}
	}
	return false
}

// distInstalled reports whether a distribution matching the import name dep
// is installed in dir, comparing case-insensitively and ignoring '-' vs '_'
// (e.g. pymysql matches PyMySQL-1.1.0.dist-info).
func distInstalled(dir, dep string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".dist-info") {
			continue
		}
		distName := strings.TrimSuffix(name, ".dist-info")
		if idx := strings.Index(distName, "-"); idx > 0 {
			distName = distName[:idx]
		}
		distName = strings.ToLower(strings.ReplaceAll(distName, "-", "_"))
		if distName == dep {
			return true
		}
	}
	return false
}
