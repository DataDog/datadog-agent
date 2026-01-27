// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// softwareTypeCargo represents software installed via Cargo (Rust package manager)
const softwareTypeCargo = "cargo"

// cargoCollector collects Rust crates installed via cargo install
// It reads from ~/.cargo/.crates.toml and ~/.cargo/.crates2.json
type cargoCollector struct{}

// cargoInstallation represents a Cargo installation directory
type cargoInstallation struct {
	cargoHome string // Path to .cargo directory
	username  string // Owner username
}

// crates2JSON represents the structure of .crates2.json
type crates2JSON struct {
	Installs map[string]crate2Install `json:"installs"`
}

// crate2Install represents a single crate installation in .crates2.json
type crate2Install struct {
	Version         string   `json:"version_req"`
	Bins            []string `json:"bins"`
	Features        []string `json:"features"`
	AllFeatures     bool     `json:"all_features"`
	NoDefaultFeatures bool   `json:"no_default_features"`
	Profile         string   `json:"profile"`
	Target          string   `json:"target"`
	Rustc           string   `json:"rustc"`
}

// getCargoInstallations finds all Cargo installation directories
func getCargoInstallations() ([]cargoInstallation, []*Warning) {
	var installations []cargoInstallation
	var warnings []*Warning

	// Per-user Cargo installations
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)

		// Default Cargo location
		cargoHome := filepath.Join(userHome, ".cargo")

		// Check for CARGO_HOME override in .bashrc/.zshrc would require parsing shell configs
		// For now, we use the default location

		// Check if .cargo exists and has installed crates
		cratesPath := filepath.Join(cargoHome, ".crates.toml")
		crates2Path := filepath.Join(cargoHome, ".crates2.json")

		if _, err := os.Stat(cratesPath); err == nil {
			installations = append(installations, cargoInstallation{
				cargoHome: cargoHome,
				username:  username,
			})
		} else if _, err := os.Stat(crates2Path); err == nil {
			installations = append(installations, cargoInstallation{
				cargoHome: cargoHome,
				username:  username,
			})
		}
	}

	return installations, warnings
}

// parseCratesToml parses the .crates.toml file
// Format:
// [v1]
// "package_name version (registry+source)" = ["bin1", "bin2"]
func parseCratesToml(path string) ([]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var crates []map[string]string
	scanner := bufio.NewScanner(file)

	// Regex to parse crate entries
	// "package_name version (registry+source)" = [...]
	crateRegex := regexp.MustCompile(`^"([^"]+)\s+([^\s]+)\s+\(([^)]+)\)"\s*=`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := crateRegex.FindStringSubmatch(line); len(matches) >= 4 {
			crate := map[string]string{
				"name":    matches[1],
				"version": matches[2],
				"source":  matches[3],
			}
			crates = append(crates, crate)
		}
	}

	return crates, scanner.Err()
}

// parseCrates2JSON parses the .crates2.json file
func parseCrates2JSON(path string) (map[string]crate2Install, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var crates crates2JSON
	if err := json.Unmarshal(data, &crates); err != nil {
		return nil, err
	}

	return crates.Installs, nil
}

// Collect enumerates Rust crates from all Cargo installations
func (c *cargoCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	// Get all Cargo installations
	installations, installWarnings := getCargoInstallations()
	warnings = append(warnings, installWarnings...)

	// If no Cargo installations found, return empty
	if len(installations) == 0 {
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Track seen crates to avoid duplicates
	seenCrates := make(map[string]bool)

	for _, install := range installations {
		// Try .crates2.json first (newer format with more info)
		crates2Path := filepath.Join(install.cargoHome, ".crates2.json")
		if crates2, err := parseCrates2JSON(crates2Path); err == nil && len(crates2) > 0 {
			for key, crateInfo := range crates2 {
				// Key format: "package_name version (source)"
				parts := strings.SplitN(key, " ", 3)
				if len(parts) < 2 {
					continue
				}
				name := parts[0]
				version := parts[1]

				// Skip if already seen
				crateKey := install.username + ":" + name
				if seenCrates[crateKey] {
					continue
				}
				seenCrates[crateKey] = true

				// Get install date from binary modification time
				var installDate string
				if len(crateInfo.Bins) > 0 {
					binPath := filepath.Join(install.cargoHome, "bin", crateInfo.Bins[0])
					if info, err := os.Stat(binPath); err == nil {
						installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
					}
				}

				// Fall back to .crates2.json modification time
				if installDate == "" {
					if info, err := os.Stat(crates2Path); err == nil {
						installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
					}
				}

				entry := &Entry{
					DisplayName: name,
					Version:     version,
					InstallDate: installDate,
					Source:      softwareTypeCargo,
					ProductCode: name,
					Status:      statusInstalled,
					Is64Bit:     is64Bit,
					InstallPath: filepath.Join(install.cargoHome, "bin"),
					UserSID:     install.username,
				}

				entries = append(entries, entry)
			}
			continue // Skip .crates.toml if .crates2.json was processed
		}

		// Fall back to .crates.toml (older format)
		cratesPath := filepath.Join(install.cargoHome, ".crates.toml")
		crates, err := parseCratesToml(cratesPath)
		if err != nil {
			warnings = append(warnings, warnf("failed to parse .crates.toml at %s: %v", cratesPath, err))
			continue
		}

		for _, crate := range crates {
			name := crate["name"]
			version := crate["version"]

			// Skip if already seen
			crateKey := install.username + ":" + name
			if seenCrates[crateKey] {
				continue
			}
			seenCrates[crateKey] = true

			// Get install date from .crates.toml modification time
			var installDate string
			if info, err := os.Stat(cratesPath); err == nil {
				installDate = info.ModTime().UTC().Format(time.RFC3339Nano)
			}

			entry := &Entry{
				DisplayName: name,
				Version:     version,
				InstallDate: installDate,
				Source:      softwareTypeCargo,
				ProductCode: name,
				Status:      statusInstalled,
				Is64Bit:     is64Bit,
				InstallPath: filepath.Join(install.cargoHome, "bin"),
				UserSID:     install.username,
			}

			entries = append(entries, entry)
		}
	}

	return entries, warnings, nil
}

