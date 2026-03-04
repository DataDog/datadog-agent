// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	// SQLite driver for MacPorts registry database
	_ "github.com/mattn/go-sqlite3"
)

// softwareTypeMacPorts represents software installed via MacPorts package manager
const softwareTypeMacPorts = "macports"

// macPortsCollector collects software installed via MacPorts package manager
// MacPorts stores its registry in a SQLite database at /opt/local/var/macports/registry/registry.db
type macPortsCollector struct{}

// isMacPortsPort64Bit returns whether the port is 64-bit from its archs value.
// archs can be a single arch (e.g. "x86_64", "arm64", "noarch", "i386") or space-separated (e.g. "x86_64 arm64").
// When archs is null or empty (e.g. older registry schema), host64Bit is used as fallback.
func isMacPortsPort64Bit(archs sql.NullString, host64Bit bool) bool {
	if !archs.Valid || archs.String == "" {
		return host64Bit
	}
	s := strings.TrimSpace(archs.String)
	// Explicit 64-bit architectures
	if strings.Contains(s, "x86_64") || strings.Contains(s, "arm64") {
		return true
	}
	// Architecture-independent; treat as 64-bit when host is 64-bit
	if s == "noarch" || strings.Contains(s, "noarch") {
		return host64Bit
	}
	// 32-bit or other (e.g. i386, ppc)
	return false
}

// Collect reads the MacPorts registry database to enumerate installed ports
func (c *macPortsCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// MacPorts standard installation paths
	// The default prefix is /opt/local, but users can customize it
	macPortsPrefixes := []string{
		"/opt/local", // Default MacPorts prefix
	}

	// Also check per-user MacPorts installations (rare but possible)
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)
	for _, userHome := range localUsers {
		// Some users install MacPorts in their home directory
		macPortsPrefixes = append(macPortsPrefixes, filepath.Join(userHome, "macports"))
		macPortsPrefixes = append(macPortsPrefixes, filepath.Join(userHome, ".macports"))
	}

	// Host 64-bit capability; used when port archs is missing or noarch
	host64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	for _, prefix := range macPortsPrefixes {
		// MacPorts registry database location
		registryPath := filepath.Join(prefix, "var", "macports", "registry", "registry.db")

		// Check if database exists
		if _, err := os.Stat(registryPath); os.IsNotExist(err) {
			continue
		}

		// Determine username for per-user installations
		var username string
		if prefix != "/opt/local" {
			// Extract username from path like /Users/username/macports
			for _, userHome := range localUsers {
				if strings.HasPrefix(prefix, userHome) {
					username = filepath.Base(userHome)
					break
				}
			}
		}

		// Open the SQLite database
		db, err := sql.Open("sqlite3", registryPath+"?mode=ro")
		if err != nil {
			warnings = append(warnings, warnf("failed to open MacPorts registry at %s: %v", registryPath, err))
			continue
		}

		// Query installed ports from the registry
		// The ports table contains: id, name, portfile, url, location, epoch, version, revision, variants, negated_variants, state, date, installtype, archs, requested, os_platform, os_major, cxx_stdlib, cxx_stdlib_overridden
		rows, err := db.Query(`
			SELECT name, version, revision, variants, date, requested, state, location, archs
			FROM ports
			WHERE state = 'installed'
		`)
		if err != nil {
			db.Close()
			warnings = append(warnings, warnf("failed to query MacPorts registry at %s: %v", registryPath, err))
			continue
		}

		for rows.Next() {
			var name, version, revision, variants, state string
			var installDate int64
			var requested int
			var location sql.NullString
			var archs sql.NullString

			if err := rows.Scan(&name, &version, &revision, &variants, &installDate, &requested, &state, &location, &archs); err != nil {
				warnings = append(warnings, warnf("failed to scan MacPorts row: %v", err))
				continue
			}

			// Build full version string (version_revision+variants)
			fullVersion := version
			if revision != "" && revision != "0" {
				fullVersion += "_" + revision
			}
			if variants != "" {
				fullVersion += variants
			}

			// Convert Unix timestamp to RFC3339
			var installDateStr string
			if installDate > 0 {
				installDateStr = time.Unix(installDate, 0).UTC().Format(time.RFC3339)
			}

			// Determine status
			status := statusInstalled
			if state != "installed" {
				status = state // Could be "imaged" or other states
			}

			// Determine install path
			installPath := filepath.Join(prefix, "var", "macports", "software", name, fullVersion)
			if location.Valid && location.String != "" {
				installPath = location.String
			}

			// Mark dependencies vs explicitly requested packages
			if requested == 0 {
				status = status + " (dependency)"
			}

			// Per-port 64-bit from archs when available; fall back to host when archs is null/empty
			entryIs64Bit := isMacPortsPort64Bit(archs, host64Bit)

			entry := &Entry{
				DisplayName: name,
				Version:     fullVersion,
				InstallDate: installDateStr,
				Source:      softwareTypeMacPorts,
				ProductCode: name, // MacPorts uses name as identifier
				Status:      status,
				Is64Bit:     entryIs64Bit,
				InstallPath: installPath,
				UserSID:     username,
			}

			entries = append(entries, entry)
		}

		if err := rows.Err(); err != nil {
			warnings = append(warnings, warnf("error iterating MacPorts registry: %v", err))
		}
		rows.Close()
		db.Close()
	}

	return entries, warnings, nil
}
