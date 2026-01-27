// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// softwareTypeNix represents software installed via Nix package manager
const softwareTypeNix = "nix"

// nixCollector collects software installed via Nix package manager
// Nix stores packages in /nix/store/ with symlinks from user profiles
type nixCollector struct{}

// nixPackage represents a parsed Nix store path
type nixPackage struct {
	Hash    string // Store path hash (e.g., "abc123def456")
	Name    string // Package name
	Version string // Package version (if parseable)
	Path    string // Full store path
}

// parseNixStorePath parses a Nix store path to extract package info
// Nix store paths have the format: /nix/store/hash-name-version
// Examples:
//   - /nix/store/abc123-git-2.42.0
//   - /nix/store/def456-nodejs-18.17.0
//   - /nix/store/ghi789-python3-3.11.5
func parseNixStorePath(storePath string) *nixPackage {
	// Extract the store entry name (everything after /nix/store/)
	base := filepath.Base(storePath)

	// Nix store entries start with a 32-character hash followed by dash
	if len(base) < 33 || base[32] != '-' {
		return nil
	}

	hash := base[:32]
	nameVersion := base[33:]

	// Try to extract name and version
	// Common patterns: name-version, name-version.ext
	// Version typically starts with a digit
	name := nameVersion
	version := ""

	// Find the last dash followed by a digit (likely version start)
	versionRegex := regexp.MustCompile(`^(.+)-(\d[^-]*)$`)
	if matches := versionRegex.FindStringSubmatch(nameVersion); len(matches) == 3 {
		name = matches[1]
		version = matches[2]
	}

	return &nixPackage{
		Hash:    hash,
		Name:    name,
		Version: version,
		Path:    storePath,
	}
}

// Collect enumerates packages from Nix profiles
// It looks at user profiles to find actively used packages (not the entire store)
func (c *nixCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Check if Nix is installed
	nixStorePath := "/nix/store"
	if _, err := os.Stat(nixStorePath); os.IsNotExist(err) {
		// Nix not installed
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Track seen packages to avoid duplicates
	seenPackages := make(map[string]bool)

	// Collect from user profiles
	// System-wide profiles
	profilePaths := []struct {
		path     string
		username string
	}{
		{"/nix/var/nix/profiles/default", ""},
		{"/run/current-system/sw", ""},
	}

	// Per-user profiles
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)
	for _, userHome := range localUsers {
		username := filepath.Base(userHome)
		// User's default profile
		profilePaths = append(profilePaths, struct {
			path     string
			username string
		}{
			filepath.Join(userHome, ".nix-profile"),
			username,
		})
		// Per-user profile in /nix/var
		profilePaths = append(profilePaths, struct {
			path     string
			username string
		}{
			filepath.Join("/nix/var/nix/profiles/per-user", username, "profile"),
			username,
		})
	}

	for _, profile := range profilePaths {
		// Resolve the profile symlink to find the actual generation
		profileTarget, err := filepath.EvalSymlinks(profile.path)
		if err != nil {
			// Profile doesn't exist or can't be resolved
			continue
		}

		// The profile typically points to a generation like:
		// /nix/var/nix/profiles/per-user/username/profile-42-link
		// which itself points to a store path

		// Look for packages in the profile's bin, lib, share directories
		// These contain symlinks to actual store paths
		dirsToScan := []string{
			filepath.Join(profileTarget, "bin"),
			filepath.Join(profileTarget, "lib"),
			filepath.Join(profileTarget, "share", "applications"),
		}

		for _, dir := range dirsToScan {
			dirEntries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			for _, entry := range dirEntries {
				entryPath := filepath.Join(dir, entry.Name())

				// Resolve symlink to find the store path
				target, err := filepath.EvalSymlinks(entryPath)
				if err != nil {
					continue
				}

				// Must be in /nix/store
				if !strings.HasPrefix(target, "/nix/store/") {
					continue
				}

				// Extract the store path (first component after /nix/store/)
				relPath, _ := filepath.Rel("/nix/store", target)
				parts := strings.SplitN(relPath, "/", 2)
				if len(parts) == 0 {
					continue
				}
				storePath := filepath.Join("/nix/store", parts[0])

				// Skip if we've already seen this package
				if seenPackages[storePath] {
					continue
				}
				seenPackages[storePath] = true

				// Parse the store path
				pkg := parseNixStorePath(storePath)
				if pkg == nil {
					continue
				}

				// Get install date from store path modification time
				var installDate string
				if info, err := os.Stat(storePath); err == nil {
					installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
				}

				softwareEntry := &Entry{
					DisplayName: pkg.Name,
					Version:     pkg.Version,
					InstallDate: installDate,
					Source:      softwareTypeNix,
					ProductCode: pkg.Hash, // Use hash as unique identifier
					Status:      statusInstalled,
					Is64Bit:     is64Bit,
					InstallPath: storePath,
					UserSID:     profile.username,
				}

				entries = append(entries, softwareEntry)
			}
		}

		// Also check manifest.nix or manifest.json for declaratively installed packages
		manifestPath := filepath.Join(profileTarget, "manifest.nix")
		if _, err := os.Stat(manifestPath); err == nil {
			// manifest.nix exists - could parse it for additional info
			// For now, we rely on symlink traversal above
		}
	}

	return entries, warnings, nil
}

