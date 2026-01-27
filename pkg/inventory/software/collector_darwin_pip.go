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

// softwareTypePip represents software installed via pip (Python package manager)
const softwareTypePip = "pip"

// pipCollector collects Python packages installed via pip
// It scans site-packages directories for .dist-info folders containing METADATA
type pipCollector struct{}

// pipSitePackages represents a Python site-packages directory
type pipSitePackages struct {
	path       string // Path to site-packages
	username   string // Owner username (empty for system-wide)
	pythonPath string // Path to the Python installation
}

// getPipSitePackages finds all Python site-packages directories
func getPipSitePackages() ([]pipSitePackages, []*Warning) {
	var sitePackages []pipSitePackages
	var warnings []*Warning

	// System-wide Python installations
	systemPythonPaths := []string{
		"/Library/Frameworks/Python.framework/Versions",     // Python.org installer
		"/usr/local/lib",                                    // Homebrew (Intel)
		"/opt/homebrew/lib",                                 // Homebrew (Apple Silicon)
		"/System/Library/Frameworks/Python.framework",       // System Python (deprecated)
		"/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions", // Xcode Python
	}

	// Find site-packages in system Python installations
	for _, basePath := range systemPythonPaths {
		// Look for pythonX.Y directories
		entries, err := os.ReadDir(basePath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Check for site-packages in common locations
			possiblePaths := []string{
				filepath.Join(basePath, entry.Name(), "lib", "python"+entry.Name(), "site-packages"),
				filepath.Join(basePath, entry.Name(), "site-packages"),
			}

			for _, spPath := range possiblePaths {
				if info, err := os.Stat(spPath); err == nil && info.IsDir() {
					sitePackages = append(sitePackages, pipSitePackages{
						path:       spPath,
						username:   "",
						pythonPath: filepath.Dir(spPath),
					})
				}
			}
		}
	}

	// Per-user Python installations and virtual environments
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)

		// User's pip packages (--user installation)
		userSitePackages := []string{
			filepath.Join(userHome, "Library", "Python"),       // macOS user site-packages
			filepath.Join(userHome, ".local", "lib"),           // Linux-style (some users use this)
			filepath.Join(userHome, ".pyenv", "versions"),      // pyenv
			filepath.Join(userHome, ".virtualenvs"),            // virtualenvwrapper
			filepath.Join(userHome, "venv"),                    // Common venv location
			filepath.Join(userHome, ".venv"),                   // Common hidden venv
			filepath.Join(userHome, "anaconda3", "lib"),        // Anaconda (overlaps with conda collector)
			filepath.Join(userHome, "miniconda3", "lib"),       // Miniconda
		}

		for _, basePath := range userSitePackages {
			// Recursively find site-packages directories
			err := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil // Skip inaccessible directories
				}

				// Limit depth to avoid scanning too deep
				rel, _ := filepath.Rel(basePath, path)
				depth := strings.Count(rel, string(filepath.Separator))
				if depth > 5 {
					return filepath.SkipDir
				}

				if d.IsDir() && d.Name() == "site-packages" {
					sitePackages = append(sitePackages, pipSitePackages{
						path:       path,
						username:   username,
						pythonPath: filepath.Dir(path),
					})
				}

				return nil
			})
			if err != nil {
				// Ignore walk errors
			}
		}
	}

	return sitePackages, warnings
}

// parsePipMetadata reads a METADATA file and extracts package information
func parsePipMetadata(path string) (name, version, author, homepage string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", "", "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// METADATA files are in email header format
		if strings.HasPrefix(line, "Name: ") {
			name = strings.TrimPrefix(line, "Name: ")
		} else if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		} else if strings.HasPrefix(line, "Author: ") {
			author = strings.TrimPrefix(line, "Author: ")
		} else if strings.HasPrefix(line, "Author-email: ") && author == "" {
			// Extract name from email format "Name <email@example.com>"
			email := strings.TrimPrefix(line, "Author-email: ")
			if idx := strings.Index(email, "<"); idx > 0 {
				author = strings.TrimSpace(email[:idx])
			}
		} else if strings.HasPrefix(line, "Home-page: ") {
			homepage = strings.TrimPrefix(line, "Home-page: ")
		} else if line == "" {
			// Empty line marks end of headers
			break
		}
	}

	return name, version, author, homepage, scanner.Err()
}

// Collect enumerates Python packages from all site-packages directories
func (c *pipCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Get all site-packages directories
	sitePackagesList, spWarnings := getPipSitePackages()
	warnings = append(warnings, spWarnings...)

	// If no site-packages found, return empty
	if len(sitePackagesList) == 0 {
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Track seen packages to avoid duplicates from the same environment
	// Key: site-packages path + package name
	seenPackages := make(map[string]bool)

	// Regex to match .dist-info directories
	distInfoRegex := regexp.MustCompile(`^(.+)-([^-]+)\.dist-info$`)

	for _, sp := range sitePackagesList {
		dirEntries, err := os.ReadDir(sp.path)
		if err != nil {
			warnings = append(warnings, warnf("failed to read site-packages at %s: %v", sp.path, err))
			continue
		}

		for _, dirEntry := range dirEntries {
			// Only process .dist-info directories
			if !dirEntry.IsDir() || !strings.HasSuffix(dirEntry.Name(), ".dist-info") {
				continue
			}

			distInfoPath := filepath.Join(sp.path, dirEntry.Name())
			metadataPath := filepath.Join(distInfoPath, "METADATA")

			// Check if METADATA file exists
			if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
				continue
			}

			// Parse package info from METADATA
			name, version, author, _, err := parsePipMetadata(metadataPath)
			if err != nil {
				// Try to extract from directory name as fallback
				if matches := distInfoRegex.FindStringSubmatch(dirEntry.Name()); len(matches) == 3 {
					name = matches[1]
					version = matches[2]
				} else {
					warnings = append(warnings, warnf("failed to parse pip metadata %s: %v", metadataPath, err))
					continue
				}
			}

			if name == "" {
				continue
			}

			// Skip if we've already seen this package in this site-packages
			packageKey := sp.path + ":" + name
			if seenPackages[packageKey] {
				continue
			}
			seenPackages[packageKey] = true

			// Get install date from .dist-info directory modification time
			var installDate string
			if info, err := os.Stat(distInfoPath); err == nil {
				installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
			}

			// Determine environment identifier from path
			envName := ""
			if strings.Contains(sp.path, "envs/") {
				// Extract environment name from path like .../envs/myenv/lib/...
				parts := strings.Split(sp.path, "/envs/")
				if len(parts) >= 2 {
					envParts := strings.SplitN(parts[1], "/", 2)
					if len(envParts) >= 1 {
						envName = envParts[0]
					}
				}
			} else if strings.Contains(sp.path, ".virtualenvs/") {
				parts := strings.Split(sp.path, ".virtualenvs/")
				if len(parts) >= 2 {
					envParts := strings.SplitN(parts[1], "/", 2)
					if len(envParts) >= 1 {
						envName = envParts[0]
					}
				}
			}

			// Add environment name to display name if in a virtual environment
			displayName := name
			if envName != "" {
				displayName = name + " [" + envName + "]"
			}

			entry := &Entry{
				DisplayName: displayName,
				Version:     version,
				InstallDate: installDate,
				Source:      softwareTypePip,
				ProductCode: name,
				Status:      statusInstalled,
				Is64Bit:     is64Bit,
				InstallPath: sp.path,
				Publisher:   author,
				UserSID:     sp.username,
			}

			entries = append(entries, entry)
		}
	}

	return entries, warnings, nil
}

