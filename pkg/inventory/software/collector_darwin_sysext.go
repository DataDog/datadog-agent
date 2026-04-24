// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"bytes"
	"encoding/xml"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// systemExtensionsCollector collects system extensions from the macOS database
type systemExtensionsCollector struct{}

// sysExtDBEntry represents an extension entry in db.plist
type sysExtDBEntry struct {
	Identifier       string
	Version          string
	BuildVersion     string
	State            string
	TeamID           string
	Categories       []string
	OriginPath       string
	StagedBundlePath string // Extracted from stagedBundleURL
}

// parseSysExtDatabase parses the /Library/SystemExtensions/db.plist database
func parseSysExtDatabase(path string) ([]sysExtDBEntry, error) {
	// Use plutil to convert to XML for parsing
	cmd := exec.Command("plutil", "-convert", "xml1", "-o", "-", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var entries []sysExtDBEntry

	// Parse the XML manually to extract extension information
	decoder := xml.NewDecoder(bytes.NewReader(output))

	var inExtensions bool
	var inExtensionDict bool
	var inBundleVersion bool
	var inStagedBundleURL bool
	var currentKey string
	var currentEntry sysExtDBEntry
	var dictDepth int
	var extensionDictDepth int // Track the depth when we entered extension dict

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "dict":
				dictDepth++
				// Extension dicts are at depth 2 inside the extensions array
				if inExtensions && !inExtensionDict && dictDepth == 2 {
					inExtensionDict = true
					extensionDictDepth = dictDepth
					currentEntry = sysExtDBEntry{}
				}
				// bundleVersion is a nested dict inside the extension dict
				if inExtensionDict && currentKey == "bundleVersion" {
					inBundleVersion = true
				}
				// stagedBundleURL is a nested dict inside the extension dict
				if inExtensionDict && currentKey == "stagedBundleURL" {
					inStagedBundleURL = true
				}
			case "array":
				if currentKey == "extensions" {
					inExtensions = true
				}
			case "key":
				var key string
				if err := decoder.DecodeElement(&key, &t); err == nil {
					currentKey = key
				}
			case "string":
				var value string
				if err := decoder.DecodeElement(&value, &t); err == nil {
					if inBundleVersion {
						switch currentKey {
						case "CFBundleShortVersionString":
							currentEntry.Version = value
						case "CFBundleVersion":
							currentEntry.BuildVersion = value
						}
					} else if inStagedBundleURL {
						// Parse the "relative" field which contains the file:// URL
						if currentKey == "relative" {
							// Convert file:// URL to path
							if strings.HasPrefix(value, "file://") {
								currentEntry.StagedBundlePath = strings.TrimPrefix(value, "file://")
								// Remove trailing slash if present
								currentEntry.StagedBundlePath = strings.TrimSuffix(currentEntry.StagedBundlePath, "/")
							}
						}
					} else if inExtensionDict && dictDepth == extensionDictDepth {
						// Only process keys at the extension dict level, not nested dicts
						switch currentKey {
						case "identifier":
							currentEntry.Identifier = value
						case "state":
							currentEntry.State = value
						case "teamID":
							currentEntry.TeamID = value
						case "originPath":
							currentEntry.OriginPath = value
						}
					}
					currentKey = ""
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "dict":
				if inBundleVersion && dictDepth == extensionDictDepth+1 {
					inBundleVersion = false
				}
				if inStagedBundleURL && dictDepth == extensionDictDepth+1 {
					inStagedBundleURL = false
				}
				if inExtensionDict && dictDepth == extensionDictDepth {
					inExtensionDict = false
					if currentEntry.Identifier != "" {
						entries = append(entries, currentEntry)
					}
				}
				dictDepth--
			case "array":
				if inExtensions && dictDepth == 1 {
					inExtensions = false
				}
			}
		}
	}

	return entries, nil
}

// Collect reads system extensions from the db.plist database
func (c *systemExtensionsCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning
	var itemsForPublisher []entryWithPath

	// System extensions database
	dbPath := "/Library/SystemExtensions/db.plist"

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return entries, warnings, nil
	}

	// Parse the database
	sysExtEntries, err := parseSysExtDatabase(dbPath)
	if err != nil {
		warnings = append(warnings, warnf("failed to parse system extensions database: %v", err))
		return entries, warnings, nil
	}

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	for _, sysExt := range sysExtEntries {
		// Use version, fall back to build version
		version := sysExt.Version
		if version == "" {
			version = sysExt.BuildVersion
		}

		// Map state to status
		status := sysExt.State
		if status == "" {
			status = "unknown"
		}

		// Get install date from the staged bundle if available
		var installDate string
		if sysExt.OriginPath != "" {
			if info, err := os.Stat(sysExt.OriginPath); err == nil {
				installDate = info.ModTime().UTC().Format(time.RFC3339)
			}
		}

		// Determine install path - prefer StagedBundlePath, fall back to OriginPath
		installPath := sysExt.StagedBundlePath
		if installPath == "" {
			installPath = sysExt.OriginPath
		}

		entry := &Entry{
			DisplayName: sysExt.Identifier,
			Version:     version,
			InstallDate: installDate,
			Source:      softwareTypeSysExt,
			ProductCode: sysExt.Identifier,
			Status:      status,
			Is64Bit:     is64Bit,
			InstallPath: installPath,
		}

		entries = append(entries, entry)

		// Use staged bundle path for publisher info if available
		if sysExt.StagedBundlePath != "" {
			itemsForPublisher = append(itemsForPublisher, entryWithPath{entry: entry, path: sysExt.StagedBundlePath})
		}
	}

	// Populate publisher info in parallel using code signing
	populatePublishersParallel(itemsForPublisher)

	return entries, warnings, nil
}
