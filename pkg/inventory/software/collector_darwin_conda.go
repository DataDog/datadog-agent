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

// softwareTypeConda represents software installed via Conda/Mamba package manager
const softwareTypeConda = "conda"

// condaCollector collects software installed via Conda or Mamba package managers
// This includes Anaconda, Miniconda, Miniforge, and Mambaforge distributions
type condaCollector struct{}

// condaPackageMeta represents the structure of conda-meta/*.json files
type condaPackageMeta struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Build         string   `json:"build"`
	BuildNumber   int      `json:"build_number"`
	Channel       string   `json:"channel"`
	Subdir        string   `json:"subdir"`
	MD5           string   `json:"md5"`
	Size          int64    `json:"size"`
	Timestamp     int64    `json:"timestamp"` // Unix timestamp in milliseconds
	Files         []string `json:"files"`
	Depends       []string `json:"depends"`
	ExtractedPath string   `json:"extracted_package_dir"`
	Requested     bool     `json:"requested_spec"` // Explicitly installed vs dependency
}

// condaEnv represents a Conda environment with its path and owner
type condaEnv struct {
	path     string // Path to the environment
	username string // Owner username (empty for system-wide)
	envName  string // Environment name
}

// getCondaEnvironments finds all Conda environments on the system
func getCondaEnvironments() ([]condaEnv, []*Warning) {
	var envs []condaEnv
	var warnings []*Warning

	// Common system-wide Conda installation paths
	systemCondaPaths := []string{
		"/opt/anaconda3",
		"/opt/miniconda3",
		"/opt/miniforge3",
		"/opt/mambaforge",
		"/usr/local/anaconda3",
		"/usr/local/miniconda3",
		// Homebrew Cask installations
		"/opt/homebrew/Caskroom/miniconda/base",
		"/opt/homebrew/Caskroom/anaconda/base",
		"/opt/homebrew/Caskroom/miniforge/base",
		"/opt/homebrew/Caskroom/mambaforge/base",
		// Intel Mac Homebrew Cask locations
		"/usr/local/Caskroom/miniconda/base",
		"/usr/local/Caskroom/anaconda/base",
		"/usr/local/Caskroom/miniforge/base",
		"/usr/local/Caskroom/mambaforge/base",
	}

	// Check system-wide installations
	for _, condaPath := range systemCondaPaths {
		if _, err := os.Stat(filepath.Join(condaPath, "conda-meta")); err == nil {
			envs = append(envs, condaEnv{path: condaPath, username: "", envName: "base"})
		}

		// Check for additional environments in envs/
		envsDir := filepath.Join(condaPath, "envs")
		if envEntries, err := os.ReadDir(envsDir); err == nil {
			for _, envEntry := range envEntries {
				if !envEntry.IsDir() {
					continue
				}
				envPath := filepath.Join(envsDir, envEntry.Name())
				if _, err := os.Stat(filepath.Join(envPath, "conda-meta")); err == nil {
					envs = append(envs, condaEnv{path: envPath, username: "", envName: envEntry.Name()})
				}
			}
		}
	}

	// Per-user Conda installations
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	// Common per-user Conda locations
	// Includes both lowercase and capitalized variants, as well as hidden directories
	userCondaDirs := []string{
		"anaconda3",
		"Anaconda3",    // Capital A variant (Windows-style installer)
		"miniconda3",
		"Miniconda3",   // Capital M variant
		"miniforge3",
		"Miniforge3",
		"mambaforge",
		"Mambaforge",
		"conda",        // Generic custom install
		".conda",       // Conda's default environment storage
		".anaconda3",   // Hidden variants
		".miniconda3",
		".miniforge3",
		".mambaforge",
		"opt/anaconda3",
		"opt/miniconda3",
	}

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)

		for _, condaDir := range userCondaDirs {
			condaPath := filepath.Join(userHome, condaDir)
			if _, err := os.Stat(filepath.Join(condaPath, "conda-meta")); err == nil {
				envs = append(envs, condaEnv{path: condaPath, username: username, envName: "base"})
			}

			// Check for additional environments
			envsDir := filepath.Join(condaPath, "envs")
			if envEntries, err := os.ReadDir(envsDir); err == nil {
				for _, envEntry := range envEntries {
					if !envEntry.IsDir() {
						continue
					}
					envPath := filepath.Join(envsDir, envEntry.Name())
					if _, err := os.Stat(filepath.Join(envPath, "conda-meta")); err == nil {
						envs = append(envs, condaEnv{path: envPath, username: username, envName: envEntry.Name()})
					}
				}
			}
		}

		// Also check .conda/envs for shared environment storage
		sharedEnvsDir := filepath.Join(userHome, ".conda", "envs")
		if envEntries, err := os.ReadDir(sharedEnvsDir); err == nil {
			for _, envEntry := range envEntries {
				if !envEntry.IsDir() {
					continue
				}
				envPath := filepath.Join(sharedEnvsDir, envEntry.Name())
				if _, err := os.Stat(filepath.Join(envPath, "conda-meta")); err == nil {
					envs = append(envs, condaEnv{path: envPath, username: username, envName: envEntry.Name()})
				}
			}
		}
	}

	return envs, warnings
}

// parseCondaMeta reads and parses a conda-meta JSON file
func parseCondaMeta(path string) (*condaPackageMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var meta condaPackageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// Collect enumerates packages from all Conda environments
func (c *condaCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Get all Conda environments
	envs, envWarnings := getCondaEnvironments()
	warnings = append(warnings, envWarnings...)

	// If no Conda environments found, return empty
	if len(envs) == 0 {
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	for _, env := range envs {
		condaMetaDir := filepath.Join(env.path, "conda-meta")

		// Read all package JSON files in conda-meta
		metaFiles, err := os.ReadDir(condaMetaDir)
		if err != nil {
			warnings = append(warnings, warnf("failed to read conda-meta at %s: %v", condaMetaDir, err))
			continue
		}

		for _, metaFile := range metaFiles {
			// Skip non-JSON files and history
			if !strings.HasSuffix(metaFile.Name(), ".json") || metaFile.Name() == "history" {
				continue
			}

			metaPath := filepath.Join(condaMetaDir, metaFile.Name())
			meta, err := parseCondaMeta(metaPath)
			if err != nil {
				warnings = append(warnings, warnf("failed to parse conda meta %s: %v", metaFile.Name(), err))
				continue
			}

			// Skip packages with no name
			if meta.Name == "" {
				continue
			}

			// Build full version string (version-build)
			fullVersion := meta.Version
			if meta.Build != "" {
				fullVersion += "-" + meta.Build
			}

			// Convert timestamp (milliseconds) to RFC3339
			var installDate string
			if meta.Timestamp > 0 {
				installDate = time.Unix(meta.Timestamp/1000, (meta.Timestamp%1000)*1000000).UTC().Format(time.RFC3339Nano)
			} else {
				// Fall back to file modification time
				if info, err := os.Stat(metaPath); err == nil {
					installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
				}
			}

			// Determine status
			status := statusInstalled
			if !meta.Requested {
				status = status + " (dependency)"
			}

			// Add environment name to distinguish packages in different envs
			displayName := meta.Name
			if env.envName != "base" {
				displayName = meta.Name + " [" + env.envName + "]"
			}

			entry := &Entry{
				DisplayName: displayName,
				Version:     fullVersion,
				InstallDate: installDate,
				Source:      softwareTypeConda,
				ProductCode: meta.Name + "@" + env.envName, // Unique per environment
				Status:      status,
				Is64Bit:     is64Bit,
				InstallPath: env.path,
				UserSID:     env.username,
			}

			// Add channel info as publisher if available
			if meta.Channel != "" && meta.Channel != "defaults" {
				entry.Publisher = meta.Channel
			}

			entries = append(entries, entry)
		}
	}

	return entries, warnings, nil
}

