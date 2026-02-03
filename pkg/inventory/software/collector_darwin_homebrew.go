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

// homebrewCollector collects software installed via Homebrew package manager
// This includes both system-wide and per-user Homebrew installations.
// Casks that install to /Applications are skipped (covered by applicationsCollector).
type homebrewCollector struct{}

// homebrewPrefix represents a Homebrew installation with associated metadata
type homebrewPrefix struct {
	path     string // Path to Homebrew prefix (e.g., /opt/homebrew)
	username string // Username (empty for system-wide)
}

// homebrewReceipt represents the structure of Homebrew's INSTALL_RECEIPT.json
type homebrewReceipt struct {
	HomebrewVersion string `json:"homebrew_version"`
	UsedOptions     []any  `json:"used_options"`
	Source          struct {
		Spec     string            `json:"spec"`
		Versions map[string]string `json:"versions"`
	} `json:"source"`
	InstalledOnRequest bool   `json:"installed_on_request"`
	InstalledAsDep     bool   `json:"installed_as_dependency"`
	Time               int64  `json:"time"` // Unix timestamp
	TabFile            string `json:"tabfile"`
	RuntimeDeps        []struct {
		FullName string `json:"full_name"`
		Version  string `json:"version"`
	} `json:"runtime_dependencies"`
	SourceModTime int64 `json:"source_modified_time"`
}

// getHomebrewPrefixes returns all Homebrew installation prefixes on the system
// This includes both system-wide installations and per-user installations.
func getHomebrewPrefixes() ([]homebrewPrefix, []*Warning) {
	var prefixes []homebrewPrefix
	var warnings []*Warning

	// System-wide Homebrew locations
	// Apple Silicon: /opt/homebrew
	// Intel: /usr/local (Homebrew files in /usr/local/Homebrew)
	systemPrefixes := []string{
		"/opt/homebrew",              // Apple Silicon
		"/usr/local",                 // Intel (Cellar is at /usr/local/Cellar)
		"/home/linuxbrew/.linuxbrew", // Linux (in case of cross-platform)
	}

	for _, prefix := range systemPrefixes {
		cellarPath := filepath.Join(prefix, "Cellar")
		if info, err := os.Stat(cellarPath); err == nil && info.IsDir() {
			prefixes = append(prefixes, homebrewPrefix{path: prefix, username: ""})
		}
	}

	// Per-user Homebrew installations
	// Users may install Homebrew in their home directories when they don't have admin access
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	// Common per-user Homebrew locations
	userBrewDirs := []string{
		".homebrew",
		"homebrew",
		".local/Homebrew",
	}

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)
		for _, brewDir := range userBrewDirs {
			prefixPath := filepath.Join(userHome, brewDir)
			cellarPath := filepath.Join(prefixPath, "Cellar")
			if info, err := os.Stat(cellarPath); err == nil && info.IsDir() {
				prefixes = append(prefixes, homebrewPrefix{path: prefixPath, username: username})
			}
		}
	}

	return prefixes, warnings
}

// parseHomebrewReceipt reads and parses an INSTALL_RECEIPT.json file
func parseHomebrewReceipt(path string) (*homebrewReceipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var receipt homebrewReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}

	return &receipt, nil
}

// Collect scans Homebrew Cellar directories for installed formulae
// It skips Casks that install to /Applications (covered by applicationsCollector).
func (c *homebrewCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Get all Homebrew prefixes (system-wide and per-user)
	prefixes, prefixWarnings := getHomebrewPrefixes()
	warnings = append(warnings, prefixWarnings...)

	// If no Homebrew installations found, return empty (not an error)
	if len(prefixes) == 0 {
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	for _, prefix := range prefixes {
		cellarPath := filepath.Join(prefix.path, "Cellar")

		// Read all formulae in the Cellar
		formulae, err := os.ReadDir(cellarPath)
		if err != nil {
			warnings = append(warnings, warnf("failed to read Homebrew Cellar at %s: %v", cellarPath, err))
			continue
		}

		for _, formula := range formulae {
			if !formula.IsDir() {
				continue
			}

			formulaName := formula.Name()
			formulaPath := filepath.Join(cellarPath, formulaName)

			// Read all versions installed for this formula
			versions, err := os.ReadDir(formulaPath)
			if err != nil {
				warnings = append(warnings, warnf("failed to read versions for %s: %v", formulaName, err))
				continue
			}

			for _, versionDir := range versions {
				if !versionDir.IsDir() {
					continue
				}

				version := versionDir.Name()
				versionPath := filepath.Join(formulaPath, version)

				// Try to read INSTALL_RECEIPT.json for additional metadata
				var installDate string
				var installedOnRequest bool
				receiptPath := filepath.Join(versionPath, "INSTALL_RECEIPT.json")
				if receipt, err := parseHomebrewReceipt(receiptPath); err == nil {
					if receipt.Time > 0 {
						installDate = time.Unix(receipt.Time, 0).UTC().Format(time.RFC3339)
					}
					installedOnRequest = receipt.InstalledOnRequest
				} else {
					// Fall back to directory modification time
					if info, err := os.Stat(versionPath); err == nil {
						installDate = info.ModTime().UTC().Format(time.RFC3339)
					}
				}

				// Determine status
				status := statusInstalled

				// Check if this is the linked (active) version
				// The active version is symlinked from opt/{formula} -> Cellar/{formula}/{version}
				optPath := filepath.Join(prefix.path, "opt", formulaName)
				if target, err := os.Readlink(optPath); err == nil {
					linkedVersion := filepath.Base(target)
					if linkedVersion != version {
						// This version is installed but not active
						status = "inactive"
					}
				}

				entry := &Entry{
					DisplayName: formulaName,
					Version:     version,
					InstallDate: installDate,
					Source:      softwareTypeHomebrew,
					ProductCode: formulaName, // Use formula name as product code
					Status:      status,
					Is64Bit:     is64Bit,
					InstallPath: versionPath,
					UserSID:     prefix.username, // Set username for per-user installs
				}

				// Add metadata about whether it was explicitly installed or as a dependency
				if !installedOnRequest {
					entry.Status = status + " (dependency)"
				}

				entries = append(entries, entry)
			}
		}

		// Also check Caskroom for Casks that don't install to /Applications
		// (e.g., fonts, drivers, prefpanes)
		caskroomPath := filepath.Join(prefix.path, "Caskroom")
		if info, err := os.Stat(caskroomPath); err == nil && info.IsDir() {
			casks, err := os.ReadDir(caskroomPath)
			if err == nil {
				for _, cask := range casks {
					if !cask.IsDir() {
						continue
					}

					caskName := cask.Name()
					caskPath := filepath.Join(caskroomPath, caskName)

					// Read versions
					caskVersions, err := os.ReadDir(caskPath)
					if err != nil {
						continue
					}

					for _, versionDir := range caskVersions {
						if !versionDir.IsDir() {
							continue
						}

						version := versionDir.Name()
						versionPath := filepath.Join(caskPath, version)

						// Check if this cask installed an app to /Applications
						// by looking for .app files in the version directory
						hasAppInApplications := false
						versionContents, err := os.ReadDir(versionPath)
						if err == nil {
							for _, content := range versionContents {
								if strings.HasSuffix(content.Name(), ".app") {
									// Check if there's a corresponding app in /Applications
									appPath := filepath.Join("/Applications", content.Name())
									if _, err := os.Stat(appPath); err == nil {
										hasAppInApplications = true
										break
									}
									// Also check user's ~/Applications
									if prefix.username != "" {
										userAppPath := filepath.Join("/Users", prefix.username, "Applications", content.Name())
										if _, err := os.Stat(userAppPath); err == nil {
											hasAppInApplications = true
											break
										}
									}
								}
							}
						}

						// Skip casks that installed apps to /Applications
						// (they're already covered by applicationsCollector)
						if hasAppInApplications {
							continue
						}

						// Get install date from directory
						var installDate string
						if info, err := os.Stat(versionPath); err == nil {
							installDate = info.ModTime().UTC().Format(time.RFC3339)
						}

						entry := &Entry{
							DisplayName: caskName + " (cask)",
							Version:     version,
							InstallDate: installDate,
							Source:      softwareTypeHomebrew,
							ProductCode: caskName,
							Status:      statusInstalled,
							Is64Bit:     is64Bit,
							InstallPath: versionPath,
							UserSID:     prefix.username,
						}

						entries = append(entries, entry)
					}
				}
			}
		}
	}

	return entries, warnings, nil
}
