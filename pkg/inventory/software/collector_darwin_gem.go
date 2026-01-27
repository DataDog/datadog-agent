// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// softwareTypeGem represents software installed via RubyGems
const softwareTypeGem = "gem"

// gemCollector collects Ruby gems installed via RubyGems
// It scans gem specifications directories for .gemspec files
type gemCollector struct{}

// gemSpecInfo represents extracted information from a gemspec file
type gemSpecInfo struct {
	Name     string
	Version  string
	Author   string
	Homepage string
	Summary  string
}

// gemInstallation represents a Ruby gems installation
type gemInstallation struct {
	specsPath string // Path to specifications directory
	username  string // Owner username (empty for system-wide)
	rubyPath  string // Path to the Ruby installation
}

// getGemInstallations finds all Ruby gem installation directories
func getGemInstallations() ([]gemInstallation, []*Warning) {
	var installations []gemInstallation
	var warnings []*Warning

	// System-wide Ruby gem locations
	systemGemPaths := []string{
		"/Library/Ruby/Gems",          // System Ruby gems
		"/usr/local/lib/ruby/gems",    // Homebrew (Intel)
		"/opt/homebrew/lib/ruby/gems", // Homebrew (Apple Silicon)
		"/opt/local/lib/ruby/gems",    // MacPorts
	}

	// Find specifications directories in system installations
	for _, basePath := range systemGemPaths {
		entries, err := os.ReadDir(basePath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			specsPath := filepath.Join(basePath, entry.Name(), "specifications")
			if info, err := os.Stat(specsPath); err == nil && info.IsDir() {
				installations = append(installations, gemInstallation{
					specsPath: specsPath,
					username:  "",
					rubyPath:  filepath.Dir(basePath),
				})
			}
		}
	}

	// Per-user Ruby gem installations
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)

		// Common per-user gem locations
		userGemPaths := []string{
			filepath.Join(userHome, ".gem", "ruby"),              // Default user gem location
			filepath.Join(userHome, ".rbenv", "versions"),        // rbenv
			filepath.Join(userHome, ".rvm", "gems"),              // RVM
			filepath.Join(userHome, ".asdf", "installs", "ruby"), // asdf
		}

		for _, basePath := range userGemPaths {
			if strings.Contains(basePath, ".rbenv") || strings.Contains(basePath, ".asdf") {
				// For rbenv/asdf, we need to scan version directories
				versionDirs, err := os.ReadDir(basePath)
				if err != nil {
					continue
				}
				for _, vDir := range versionDirs {
					if !vDir.IsDir() {
						continue
					}
					specsPath := filepath.Join(basePath, vDir.Name(), "lib", "ruby", "gems")
					// Find the actual gem version directory
					gemVersionDirs, err := os.ReadDir(specsPath)
					if err != nil {
						continue
					}
					for _, gvDir := range gemVersionDirs {
						if !gvDir.IsDir() {
							continue
						}
						actualSpecsPath := filepath.Join(specsPath, gvDir.Name(), "specifications")
						if info, err := os.Stat(actualSpecsPath); err == nil && info.IsDir() {
							installations = append(installations, gemInstallation{
								specsPath: actualSpecsPath,
								username:  username,
								rubyPath:  filepath.Join(basePath, vDir.Name()),
							})
						}
					}
				}
			} else if strings.Contains(basePath, ".rvm") {
				// RVM has a different structure: .rvm/gems/ruby-version/
				entries, err := os.ReadDir(basePath)
				if err != nil {
					continue
				}
				for _, entry := range entries {
					if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "ruby-") {
						continue
					}
					specsPath := filepath.Join(basePath, entry.Name(), "specifications")
					if info, err := os.Stat(specsPath); err == nil && info.IsDir() {
						installations = append(installations, gemInstallation{
							specsPath: specsPath,
							username:  username,
							rubyPath:  filepath.Join(basePath, entry.Name()),
						})
					}
				}
			} else {
				// Default .gem/ruby/version structure
				entries, err := os.ReadDir(basePath)
				if err != nil {
					continue
				}
				for _, entry := range entries {
					if !entry.IsDir() {
						continue
					}
					specsPath := filepath.Join(basePath, entry.Name(), "specifications")
					if info, err := os.Stat(specsPath); err == nil && info.IsDir() {
						installations = append(installations, gemInstallation{
							specsPath: specsPath,
							username:  username,
							rubyPath:  filepath.Dir(basePath),
						})
					}
				}
			}
		}
	}

	return installations, warnings
}

// parseGemspec parses a .gemspec file and extracts basic information
// Note: gemspec files are Ruby code, so we use regex-based extraction
func parseGemspec(path string) (*gemSpecInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info := &gemSpecInfo{}
	scanner := bufio.NewScanner(file)

	// Patterns to match common gemspec attributes
	// e.g., s.name = "gemname" or spec.name = 'gemname'
	nameRegex := regexp.MustCompile(`\.name\s*=\s*["']([^"']+)["']`)
	versionRegex := regexp.MustCompile(`\.version\s*=\s*["']([^"']+)["']`)
	authorRegex := regexp.MustCompile(`\.authors?\s*=\s*\[?\s*["']([^"'\]]+)["']`)
	homepageRegex := regexp.MustCompile(`\.homepage\s*=\s*["']([^"']+)["']`)
	summaryRegex := regexp.MustCompile(`\.summary\s*=\s*["']([^"']+)["']`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := nameRegex.FindStringSubmatch(line); len(matches) == 2 && info.Name == "" {
			info.Name = matches[1]
		}
		if matches := versionRegex.FindStringSubmatch(line); len(matches) == 2 && info.Version == "" {
			info.Version = matches[1]
		}
		if matches := authorRegex.FindStringSubmatch(line); len(matches) == 2 && info.Author == "" {
			info.Author = matches[1]
		}
		if matches := homepageRegex.FindStringSubmatch(line); len(matches) == 2 && info.Homepage == "" {
			info.Homepage = matches[1]
		}
		if matches := summaryRegex.FindStringSubmatch(line); len(matches) == 2 && info.Summary == "" {
			info.Summary = matches[1]
		}
	}

	return info, scanner.Err()
}

// Collect enumerates Ruby gems from all gem installations
func (c *gemCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Get all gem installations
	installations, installWarnings := getGemInstallations()
	warnings = append(warnings, installWarnings...)

	// If no gem installations found, return empty
	if len(installations) == 0 {
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Track seen gems to avoid duplicates
	seenGems := make(map[string]bool)

	// Regex to extract name and version from gemspec filename
	// Format: name-version.gemspec
	gemspecNameRegex := regexp.MustCompile(`^(.+)-([^-]+)\.gemspec$`)

	for _, install := range installations {
		dirEntries, err := os.ReadDir(install.specsPath)
		if err != nil {
			warnings = append(warnings, warnf("failed to read gem specifications at %s: %v", install.specsPath, err))
			continue
		}

		for _, dirEntry := range dirEntries {
			// Only process .gemspec files
			if !strings.HasSuffix(dirEntry.Name(), ".gemspec") {
				continue
			}

			gemspecPath := filepath.Join(install.specsPath, dirEntry.Name())

			// Try to extract name and version from filename first
			var name, version string
			if matches := gemspecNameRegex.FindStringSubmatch(dirEntry.Name()); len(matches) == 3 {
				name = matches[1]
				version = matches[2]
			}

			// Parse gemspec for additional info
			info, err := parseGemspec(gemspecPath)
			if err != nil {
				// Use filename-derived info if parsing fails
				if name == "" {
					warnings = append(warnings, warnf("failed to parse gemspec %s: %v", dirEntry.Name(), err))
					continue
				}
			} else {
				// Prefer parsed info
				if info.Name != "" {
					name = info.Name
				}
				if info.Version != "" {
					version = info.Version
				}
			}

			if name == "" {
				continue
			}

			// Skip if already seen this gem for this user
			gemKey := install.username + ":" + name + ":" + version
			if seenGems[gemKey] {
				continue
			}
			seenGems[gemKey] = true

			// Get install date from gemspec modification time
			var installDate string
			if fileInfo, err := os.Stat(gemspecPath); err == nil {
				installDate = fileInfo.ModTime().UTC().Format(time.RFC3339Nano)
			}

			entry := &Entry{
				DisplayName: name,
				Version:     version,
				InstallDate: installDate,
				Source:      softwareTypeGem,
				ProductCode: name,
				Status:      statusInstalled,
				Is64Bit:     is64Bit,
				InstallPath: filepath.Dir(install.specsPath), // Parent of specifications
				UserSID:     install.username,
			}

			// Add author as publisher if available
			if info != nil && info.Author != "" {
				entry.Publisher = info.Author
			}

			entries = append(entries, entry)
		}
	}

	return entries, warnings, nil
}
