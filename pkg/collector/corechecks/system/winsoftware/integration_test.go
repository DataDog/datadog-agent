// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winsoftware

import (
	"bytes"
	"encoding/csv"
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

	// Filter only MSI packages
	filterMsi := func(software *SoftwareEntry) bool {
		return strings.Contains(software.Source, "msi")
	}

	// Shows different ways to get the software inventory.
	// This shows that the Programs provider for Get-Package is entirely based on the registry keys.
	for _, tt := range []struct {
		name     string
		cmd      string
		filterFn func(software *SoftwareEntry) bool
	}{
		{
			name: "Test against Get-Package with Programs provider",
			cmd: `$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
			Get-Package -AllVersions -IncludeWindowsInstaller -IncludeSystemComponent |
			Select-Object Name, Version, FastPackageReference |
			Sort-Object Name |
			ConvertTo-Csv -NoTypeInformation`,
		},
		{
			name: "Test against Get-Package with MSI provider",
			cmd: `$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
			Get-Package -AllVersions -ProviderName msi |
			Select-Object Name, Version, FastPackageReference |
			Sort-Object Name |
			ConvertTo-Csv -NoTypeInformation`,
			filterFn: filterMsi, // Only include MSI packages since we want to compare with the msi provider
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
			psInventory := make(map[string][]SoftwareEntry)

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
						psInventory[name] = []SoftwareEntry{{
							DisplayName: name,
							Version:     version,
							Properties:  map[string]string{"ProductCode": productCode},
						}}
					} else {
						psInventory[name] = append(psInventory[name], SoftwareEntry{
							DisplayName: name,
							Version:     version,
							Properties:  map[string]string{"ProductCode": productCode},
						})
					}
				}
			}

			// Get our inventory
			ourInventory, warnings, err := GetSoftwareInventory()

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
			ourSoftwareMap := make(map[string][]*SoftwareEntry)
			for _, software := range ourInventory {
				if tt.filterFn != nil && !tt.filterFn(software) {
					continue // Skip if filter function is provided and returns false
				}

				if _, exists := ourSoftwareMap[software.DisplayName]; !exists {
					ourSoftwareMap[software.DisplayName] = []*SoftwareEntry{software}
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
								name, psEntry.Properties[msiProductCode], psEntry.Version))
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
								name, psEntry.Properties[msiProductCode], psEntry.Version))
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
								name, entry.Properties[msiProductCode], entry.Version, entry.Source))
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
								name, entry.Properties[msiProductCode], entry.Version, entry.Source))
					}
				}
			}

			// Log results
			if len(missingFromOurs) > 0 {
				t.Errorf("Missing %d software entries that PowerShell found:\n%s",
					len(missingFromOurs), strings.Join(missingFromOurs, "\n"))
			}
			if len(extraInOurs) > 0 {
				t.Logf("Found %d extra software entries:\n%s",
					len(extraInOurs), strings.Join(extraInOurs, "\n"))
			}
		})
	}
}
