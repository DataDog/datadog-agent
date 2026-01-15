// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegrationCompareWithPowerShell(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Shows different ways to get the software inventory.
	// This shows that the Programs provider for Get-Package is entirely based on the registry keys.
	for _, tt := range []struct {
		name        string
		cmd         string
		collectorFn func() ([]*Entry, []*Warning, error)
	}{
		{
			name: "Test against Get-Package with Programs provider",
			cmd: `$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
			Get-Package -AllVersions -IncludeWindowsInstaller -IncludeSystemComponent |
			Select-Object Name, Version, FastPackageReference |
			Sort-Object Name |
			ConvertTo-Csv -NoTypeInformation`,
			collectorFn: GetSoftwareInventory,
		},
		{
			name: "Test against Get-Package with MSI provider",
			cmd: `$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
			Get-Package -AllVersions -ProviderName msi |
			Select-Object Name, Version, FastPackageReference |
			Sort-Object Name |
			ConvertTo-Csv -NoTypeInformation`,
			collectorFn: func() ([]*Entry, []*Warning, error) {
				return GetSoftwareInventoryWithCollectors([]Collector{&mSICollector{}})
			},
		},
		{
			name: "Test against regular Registry collection",
			cmd: `$ErrorActionPreference = "Stop"
			$items = @()
			$uninstallKeys = @(
				"HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*",
				"HKLM:\SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*"
			)
			foreach ($key in $uninstallKeys) {
				$items += Get-ItemProperty $key -ErrorAction SilentlyContinue |
					Where-Object { $_.DisplayName -and $_.DisplayName.Trim() } |
					Select-Object DisplayName, DisplayVersion, Publisher, InstallDate
			}
			$items | Sort-Object DisplayName | ConvertTo-Csv -NoTypeInformation`,
			collectorFn: func() ([]*Entry, []*Warning, error) {
				return GetSoftwareInventoryWithCollectors([]Collector{&registryCollector{}})
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Get PowerShell inventory to compare
			cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", tt.cmd)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			require.NoError(t, err, "PowerShell command failed: %s", stderr.String())

			// Parse CSV output into a nested map: name -> []SoftwareEntry
			psInventory := make(map[string][]Entry)

			// Create a new reader with UTF-8 BOM handling
			csvBytes := stdout.Bytes()
			if len(csvBytes) >= 3 && csvBytes[0] == 0xEF && csvBytes[1] == 0xBB && csvBytes[2] == 0xBF {
				// Skip UTF-8 BOM if present
				csvBytes = csvBytes[3:]
			}
			reader := csv.NewReader(bytes.NewReader(csvBytes))
			records, err := reader.ReadAll()
			require.NoError(t, err, "Failed to parse CSV output")

			// Skip header row and build map
			for i := 1; i < len(records); i++ {
				if len(records[i]) >= 3 && records[i][0] != "" { // Name, Version, FastPackageReference
					name := records[i][0]
					version := trimVersion(records[i][1])
					productCode := records[i][2]

					if _, exists := psInventory[name]; !exists {
						psInventory[name] = []Entry{{
							DisplayName: name,
							Version:     version,
							ProductCode: productCode,
						}}
					} else {
						psInventory[name] = append(psInventory[name], Entry{
							DisplayName: name,
							Version:     version,
							ProductCode: productCode,
						})
					}
				}
			}

			// Get our inventory
			ourInventory, warnings, err := tt.collectorFn()

			require.NoError(t, err)
			if len(warnings) > 0 {
				for _, w := range warnings {
					t.Logf("Warning: %s", w.Message)
				}
			}

			// Sort inventory by DisplayName (case-insensitive)
			// Not necessary for testing but makes output easier to debug
			sort.Slice(ourInventory, func(i, j int) bool {
				return strings.ToLower(ourInventory[i].DisplayName) < strings.ToLower(ourInventory[j].DisplayName)
			})

			// Build comparable nested map from our inventory
			// Map: name -> []SoftwareEntry
			// This allows us to compare versions and sources efficiently
			ourSoftwareMap := make(map[string][]*Entry)
			for _, software := range ourInventory {
				if _, exists := ourSoftwareMap[software.DisplayName]; !exists {
					ourSoftwareMap[software.DisplayName] = []*Entry{software}
				} else {
					ourSoftwareMap[software.DisplayName] = append(ourSoftwareMap[software.DisplayName], software)
				}
			}

			// Compare inventories
			var missingFromOurs []string
			var extraInOurs []string

			// Check what PowerShell has that we don't (critical - test should fail)
			for name, psVersions := range psInventory {
				ourVersions, exists := ourSoftwareMap[name]
				if !exists {
					// We completely missed this software
					for _, psEntry := range psVersions {
						missingFromOurs = append(missingFromOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s)",
								name, psEntry.ProductCode, psEntry.Version))
					}
					continue
				}

				// Software exists but check if we have all versions
				for _, psEntry := range psVersions {
					found := false
					for _, ourEntry := range ourVersions {
						if psEntry.Version == ourEntry.Version {
							found = true
							break
						}
					}
					if !found {
						missingFromOurs = append(missingFromOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s)",
								name, psEntry.ProductCode, psEntry.Version))
					}
				}
			}

			// Check what we have that PowerShell doesn't (good - we found more)
			for name, ourVersions := range ourSoftwareMap {
				psVersion, exists := psInventory[name]
				if !exists {
					// PowerShell completely missed this software
					for _, entry := range ourVersions {
						extraInOurs = append(extraInOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s, Source: %s)",
								name, entry.ProductCode, entry.Version, entry.Source))
					}
					continue
				}

				// Compare versions if PowerShell found the software
				for _, entry := range ourVersions {
					found := false
					for _, psEntry := range psVersion {
						if entry.Version == psEntry.Version {
							found = true
							break
						}
					}
					// We found a version that PowerShell didn't have
					if !found {
						extraInOurs = append(extraInOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s, Source: %s)",
								name, entry.ProductCode, entry.Version, entry.Source))
					}
				}
			}

			// Log results
			if len(missingFromOurs) > 0 {
				// Check if we have privilege warnings indicating we couldn't access user hives
				hasHiveMountFailure := false
				for _, w := range warnings {
					if strings.Contains(w.Message, "failed to mount hive") {
						hasHiveMountFailure = true
						break
					}
				}

				if hasHiveMountFailure {
					// Filter missing entries - we expect to miss HKCU entries when we can't mount hives
					var hkcuMissing, otherMissing []string
					for _, missing := range missingFromOurs {
						if strings.Contains(missing, "hkcu32\\") || strings.Contains(missing, "hkcu64\\") {
							hkcuMissing = append(hkcuMissing, missing)
						} else {
							otherMissing = append(otherMissing, missing)
						}
					}

					if len(hkcuMissing) > 0 {
						t.Logf("Note: Could not verify %d HKCU entries due to insufficient privileges to mount user hives (expected in containers). Missing entries:\n%s",
							len(hkcuMissing), strings.Join(hkcuMissing, "\n"))
					}

					if len(otherMissing) > 0 {
						t.Errorf("Missing %d software entries that PowerShell found:\n%s",
							len(otherMissing), strings.Join(otherMissing, "\n"))
					}
				} else {
					t.Errorf("Missing %d software entries that PowerShell found:\n%s",
						len(missingFromOurs), strings.Join(missingFromOurs, "\n"))
				}
			}
			if len(extraInOurs) > 0 {
				t.Logf("Found %d extra software entries:\n%s",
					len(extraInOurs), strings.Join(extraInOurs, "\n"))
			}
		})
	}
}

func TestIntegrationMSStoreApps(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Define struct to match PowerShell JSON output
	type psAppxPackage struct {
		Name              string
		Version           string
		Publisher         string
		Architecture      int
		PackageFamilyName string
	}

	// Get PowerShell MS Store packages (doesn't include apps within the package)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", `
		$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
		$apps = Get-AppxPackage -AllUsers |
		Where-Object {
			-not $_.IsFramework -and
			-not $_.IsResourcePackage -and
			-not $_.IsOptional -and
			-not $_.IsBundle
		} |
		Select-Object Name, Version, Publisher, Architecture, PackageFamilyName |
		Sort-Object Name;
		if ($apps) {
			$apps | ConvertTo-Json -AsArray  # Force array output even for single item
		} else {
			Write-Output "[]"  # Return empty array if no apps
		}
	`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, err, "PowerShell command failed: %s", stderr.String())

	// Parse JSON output
	var psApps []psAppxPackage
	jsonBytes := stdout.Bytes()
	// Skip UTF-8 BOM if present
	if len(jsonBytes) >= 3 && jsonBytes[0] == 0xEF && jsonBytes[1] == 0xBB && jsonBytes[2] == 0xBF {
		jsonBytes = jsonBytes[3:]
	}
	if len(jsonBytes) > 0 {
		err = json.Unmarshal(jsonBytes, &psApps)
		require.NoError(t, err, "Failed to parse JSON output")
	}
	// Build map: productCode -> []Entry
	psInventory := []Entry{}
	for _, app := range psApps {
		is64bit := app.Architecture == 9

		entry := Entry{
			DisplayName: app.Name,
			Version:     app.Version,
			Publisher:   app.Publisher,
			ProductCode: app.PackageFamilyName,
			Is64Bit:     is64bit,
		}

		psInventory = append(psInventory, entry)
	}

	// Get our MS Store apps inventory
	collector := &msStoreAppsCollector{}
	ourInventory, warnings, err := collector.Collect()
	require.NoError(t, err)
	if len(warnings) > 0 {
		for _, w := range warnings {
			t.Logf("Warning: %s", w.Message)
		}
	}

	// Build comparable map from our inventory: productCode -> []Entry
	// This will group apps together by package using the ProductCode as the key
	// so we can only compare packages with the PS output.
	ourSoftwareMap := make(map[string][]*Entry)
	for _, software := range ourInventory {
		if _, exists := ourSoftwareMap[software.ProductCode]; !exists {
			ourSoftwareMap[software.ProductCode] = []*Entry{software}
		} else {
			ourSoftwareMap[software.ProductCode] = append(ourSoftwareMap[software.ProductCode], software)
		}
	}

	// Verify that everything from PowerShell exists in our collector with matching fields.
	// We only check one direction: PS -> Ours. This is because the PS command doesn't include apps in each package.
	// Technically, we are only checking if the packages exists in our collector, not apps within the package.
	// At least, this ensures we don't miss any apps that PowerShell can detect.
	var missingOrMismatchedApps []string

	for _, psEntry := range psInventory {
		ourPackages, exists := ourSoftwareMap[psEntry.ProductCode]
		if !exists {
			// We completely missed this package
			missingOrMismatchedApps = append(missingOrMismatchedApps,
				fmt.Sprintf("MISSING: %s v%s (ProductCode: %s)",
					psEntry.DisplayName, psEntry.Version, psEntry.ProductCode))
			continue
		}

		found := false
		for _, ourEntry := range ourPackages {
			if psEntry.Version == ourEntry.Version {
				// Found matching version, now verify fields match
				if psEntry.Publisher != ourEntry.Publisher {
					missingOrMismatchedApps = append(missingOrMismatchedApps,
						fmt.Sprintf("MISMATCH: %s v%s - Publisher differs (PS: %s, Ours: %s)",
							psEntry.DisplayName, psEntry.Version, psEntry.Publisher, ourEntry.Publisher))
				}
				if psEntry.Is64Bit != ourEntry.Is64Bit {
					missingOrMismatchedApps = append(missingOrMismatchedApps,
						fmt.Sprintf("MISMATCH: %s v%s - Is64Bit differs (PS: %v, Ours: %v)",
							psEntry.DisplayName, psEntry.Version, psEntry.Is64Bit, ourEntry.Is64Bit))
				}
				found = true
				break
			}
		}
		if !found {
			missingOrMismatchedApps = append(missingOrMismatchedApps,
				fmt.Sprintf("MISSING VERSION: %s v%s (ProductCode: %s)",
					psEntry.DisplayName, psEntry.Version, psEntry.ProductCode))
		}

	}

	// Log results
	t.Logf("PowerShell found %d MS Store apps", len(psInventory))
	t.Logf("Our collector found %d MS Store apps", len(ourSoftwareMap))

	if len(missingOrMismatchedApps) > 0 {
		t.Errorf("Found %d missing or mismatched MS Store apps:\n%s",
			len(missingOrMismatchedApps), strings.Join(missingOrMismatchedApps, "\n"))
	}
}
