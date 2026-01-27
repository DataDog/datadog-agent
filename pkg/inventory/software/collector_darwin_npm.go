// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// softwareTypeNpm represents software installed via npm (Node.js package manager)
const softwareTypeNpm = "npm"

// npmCollector collects globally installed npm packages
// It scans global node_modules directories for package.json files
type npmCollector struct{}

// npmPackageJSON represents the structure of a package.json file
type npmPackageJSON struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      interface{}       `json:"author"` // Can be string or object
	Homepage    string            `json:"homepage"`
	License     string            `json:"license"`
	Repository  interface{}       `json:"repository"` // Can be string or object
	Bin         interface{}       `json:"bin"`        // Can be string or object
	Scripts     map[string]string `json:"scripts"`
}

// npmGlobalPrefix represents a global npm installation prefix
type npmGlobalPrefix struct {
	path     string // Path to node_modules
	username string // Owner username (empty for system-wide)
}

// getNpmGlobalPrefixes finds all global npm installation directories
func getNpmGlobalPrefixes() ([]npmGlobalPrefix, []*Warning) {
	var prefixes []npmGlobalPrefix
	var warnings []*Warning

	// System-wide npm global locations
	systemPrefixes := []string{
		"/usr/local/lib/node_modules",      // Default global location (Intel Mac, npm installed via pkg)
		"/opt/homebrew/lib/node_modules",   // Homebrew (Apple Silicon)
		"/usr/local/lib/node_modules",      // Homebrew (Intel)
		"/opt/local/lib/node_modules",      // MacPorts
	}

	for _, prefix := range systemPrefixes {
		if info, err := os.Stat(prefix); err == nil && info.IsDir() {
			prefixes = append(prefixes, npmGlobalPrefix{path: prefix, username: ""})
		}
	}

	// Per-user npm global installations
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)

		// Common per-user npm global locations
		userPrefixes := []string{
			// Default location when using `npm config set prefix ~/.npm-global`
			filepath.Join(userHome, ".npm-global", "lib", "node_modules"),
			// nvm (Node Version Manager) global packages
			filepath.Join(userHome, ".nvm", "versions", "node"),
			// fnm (Fast Node Manager)
			filepath.Join(userHome, ".fnm", "node-versions"),
			// n (Node version manager)
			filepath.Join(userHome, "n", "lib", "node_modules"),
			// volta
			filepath.Join(userHome, ".volta", "tools", "image", "packages"),
		}

		for _, prefix := range userPrefixes {
			if strings.Contains(prefix, ".nvm") || strings.Contains(prefix, ".fnm") {
				// For nvm/fnm, we need to scan version directories
				versionDirs, err := os.ReadDir(prefix)
				if err != nil {
					continue
				}
				for _, vDir := range versionDirs {
					if !vDir.IsDir() {
						continue
					}
					nodeModulesPath := filepath.Join(prefix, vDir.Name(), "lib", "node_modules")
					if info, err := os.Stat(nodeModulesPath); err == nil && info.IsDir() {
						prefixes = append(prefixes, npmGlobalPrefix{path: nodeModulesPath, username: username})
					}
				}
			} else if strings.Contains(prefix, ".volta") {
				// Volta stores packages differently
				if info, err := os.Stat(prefix); err == nil && info.IsDir() {
					prefixes = append(prefixes, npmGlobalPrefix{path: prefix, username: username})
				}
			} else {
				if info, err := os.Stat(prefix); err == nil && info.IsDir() {
					prefixes = append(prefixes, npmGlobalPrefix{path: prefix, username: username})
				}
			}
		}
	}

	return prefixes, warnings
}

// parseNpmPackageJSON reads and parses a package.json file
func parseNpmPackageJSON(path string) (*npmPackageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg npmPackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}

// extractAuthor extracts author name from the Author field which can be string or object
func extractAuthor(author interface{}) string {
	switch v := author.(type) {
	case string:
		// "Author Name <email> (url)" format
		if idx := strings.Index(v, "<"); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		if idx := strings.Index(v, "("); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		return v
	case map[string]interface{}:
		if name, ok := v["name"].(string); ok {
			return name
		}
	}
	return ""
}

// Collect enumerates globally installed npm packages
func (c *npmCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Get all global npm prefixes
	prefixes, prefixWarnings := getNpmGlobalPrefixes()
	warnings = append(warnings, prefixWarnings...)

	// If no npm global directories found, return empty
	if len(prefixes) == 0 {
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Track seen packages to avoid duplicates
	seenPackages := make(map[string]bool)

	for _, prefix := range prefixes {
		dirEntries, err := os.ReadDir(prefix.path)
		if err != nil {
			warnings = append(warnings, warnf("failed to read npm node_modules at %s: %v", prefix.path, err))
			continue
		}

		for _, dirEntry := range dirEntries {
			if !dirEntry.IsDir() {
				continue
			}

			pkgName := dirEntry.Name()

			// Skip hidden directories and npm internal packages
			if strings.HasPrefix(pkgName, ".") {
				continue
			}

			// Handle scoped packages (@org/package)
			if strings.HasPrefix(pkgName, "@") {
				scopedPath := filepath.Join(prefix.path, pkgName)
				scopedEntries, err := os.ReadDir(scopedPath)
				if err != nil {
					continue
				}

				for _, scopedEntry := range scopedEntries {
					if !scopedEntry.IsDir() {
						continue
					}

					fullPkgName := pkgName + "/" + scopedEntry.Name()
					pkgPath := filepath.Join(scopedPath, scopedEntry.Name())
					entry := c.createEntryFromPackage(pkgPath, fullPkgName, prefix.username, is64Bit, seenPackages, &warnings)
					if entry != nil {
						entries = append(entries, entry)
					}
				}
			} else {
				pkgPath := filepath.Join(prefix.path, pkgName)
				entry := c.createEntryFromPackage(pkgPath, pkgName, prefix.username, is64Bit, seenPackages, &warnings)
				if entry != nil {
					entries = append(entries, entry)
				}
			}
		}
	}

	return entries, warnings, nil
}

// createEntryFromPackage creates an Entry from an npm package directory
func (c *npmCollector) createEntryFromPackage(pkgPath, pkgName, username string, is64Bit bool, seenPackages map[string]bool, warnings *[]*Warning) *Entry {
	packageJSONPath := filepath.Join(pkgPath, "package.json")

	// Check if package.json exists
	if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
		return nil
	}

	// Parse package.json
	pkg, err := parseNpmPackageJSON(packageJSONPath)
	if err != nil {
		*warnings = append(*warnings, warnf("failed to parse npm package.json at %s: %v", packageJSONPath, err))
		return nil
	}

	// Use name from package.json, fall back to directory name
	name := pkg.Name
	if name == "" {
		name = pkgName
	}

	// Skip if already seen
	packageKey := username + ":" + name
	if seenPackages[packageKey] {
		return nil
	}
	seenPackages[packageKey] = true

	// Get install date from package.json modification time
	var installDate string
	if info, err := os.Stat(packageJSONPath); err == nil {
		installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
	}

	entry := &Entry{
		DisplayName: name,
		Version:     pkg.Version,
		InstallDate: installDate,
		Source:      softwareTypeNpm,
		ProductCode: name,
		Status:      statusInstalled,
		Is64Bit:     is64Bit,
		InstallPath: pkgPath,
		Publisher:   extractAuthor(pkg.Author),
		UserSID:     username,
	}

	return entry
}

