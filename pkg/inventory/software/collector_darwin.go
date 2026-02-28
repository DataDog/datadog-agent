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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Software types for macOS
const (
	// softwareTypeApp represents applications from /Applications
	softwareTypeApp = "app"
	// softwareTypeSystemApp represents Apple system applications from /System/Applications
	softwareTypeSystemApp = "system_app"
	// softwareTypePkg represents software installed via PKG installer
	softwareTypePkg = "pkg"
	// softwareTypeMAS represents applications from the Mac App Store
	softwareTypeMAS = "mas"
	// softwareTypeKext represents kernel extensions
	softwareTypeKext = "kext"
	// softwareTypeSysExt represents system extensions (modern replacement for kexts)
	softwareTypeSysExt = "sysext"
	// softwareTypeHomebrew represents software installed via Homebrew package manager
	softwareTypeHomebrew = "homebrew"
)

// Install source values for macOS applications
// These indicate how an application was installed on the system
const (
	// installSourcePkg indicates the app was installed via a .pkg installer package
	installSourcePkg = "pkg"
	// installSourceMAS indicates the app was installed from the Mac App Store
	installSourceMAS = "mas"
	// installSourceManual indicates the app was installed manually (drag-and-drop, etc.)
	installSourceManual = "manual"
)

// defaultCollectors returns the default collectors for production use on macOS
// These collectors focus on system-level software relevant to IT professionals:
// - Applications (.app bundles)
// - PKG installer receipts
// - Kernel extensions (kexts)
// - System extensions
// - Homebrew packages
// - MacPorts packages
func defaultCollectors() []Collector {
	return []Collector{
		&applicationsCollector{},
		&pkgReceiptsCollector{},
		&kernelExtensionsCollector{},
		&systemExtensionsCollector{},
		&homebrewCollector{},
		&macPortsCollector{},
	}
}

// parsePlistToMap parses plist XML data into a map
func parsePlistToMap(data []byte) (map[string]string, error) {
	// Simple plist parser that extracts key-string pairs
	result := make(map[string]string)

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var currentKey string
	var inDict bool

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "dict":
				inDict = true
			case "key":
				if inDict {
					var key string
					if err := decoder.DecodeElement(&key, &t); err == nil {
						currentKey = key
					}
				}
			case "string":
				if inDict && currentKey != "" {
					var value string
					if err := decoder.DecodeElement(&value, &t); err == nil {
						result[currentKey] = value
					}
					currentKey = ""
				}
			case "date":
				if inDict && currentKey != "" {
					var value string
					if err := decoder.DecodeElement(&value, &t); err == nil {
						result[currentKey] = value
					}
					currentKey = ""
				}
			default:
				// Skip other value types and reset current key
				if inDict && currentKey != "" {
					currentKey = ""
				}
			}
		case xml.EndElement:
			if t.Name.Local == "dict" {
				// Only process the first dict level
				break
			}
		}
	}

	return result, nil
}

// readPlistFile reads a plist file and returns its contents as a map
// It handles both XML and binary plist formats
func readPlistFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Check if it's a binary plist (starts with "bplist")
	if bytes.HasPrefix(data, []byte("bplist")) {
		// Convert binary plist to XML using plutil
		cmd := exec.Command("plutil", "-convert", "xml1", "-o", "-", path)
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		data = output
	}

	return parsePlistToMap(data)
}

// Status constants for broken state detection
const (
	statusInstalled = "installed"
	statusBroken    = "broken"
)

// checkAppBundleIntegrity verifies that an app bundle has its required executable.
// Returns an empty string if the bundle is OK, or a reason string if it's broken.
func checkAppBundleIntegrity(appPath string, plistData map[string]string) string {
	// Get the executable name from Info.plist
	executableName := plistData["CFBundleExecutable"]
	if executableName == "" {
		// If no executable specified, the bundle is incomplete
		return "Info.plist missing CFBundleExecutable"
	}

	// Check if the executable exists
	executablePath := filepath.Join(appPath, "Contents", "MacOS", executableName)
	if _, err := os.Stat(executablePath); os.IsNotExist(err) {
		return "executable not found: Contents/MacOS/" + executableName
	}

	return "" // Bundle is OK
}

// checkKextBundleIntegrity verifies that a kext bundle has its required executable.
// Returns an empty string if the bundle is OK, or a reason string if it's broken.
func checkKextBundleIntegrity(kextPath string, plistData map[string]string) string {
	// Get the executable name from Info.plist
	executableName := plistData["CFBundleExecutable"]
	if executableName == "" {
		// Some kexts may not have an executable (codeless kexts)
		// Check if it has a MacOS directory with any executable
		macOSDir := filepath.Join(kextPath, "Contents", "MacOS")
		if _, err := os.Stat(macOSDir); os.IsNotExist(err) {
			// No MacOS directory - might be a codeless kext, consider it OK
			return ""
		}
	}

	// Check if the executable exists
	executablePath := filepath.Join(kextPath, "Contents", "MacOS", executableName)
	if _, err := os.Stat(executablePath); os.IsNotExist(err) {
		return "executable not found: Contents/MacOS/" + executableName
	}

	return "" // Bundle is OK
}

// companySuffixes contains common company name suffixes used to identify corporate entities
// Note: "Company" is intentionally excluded as it's too generic and causes false matches
// (e.g., "is a Datadog company" would match incorrectly)
var companySuffixes = []string{
	"Inc.", "Inc", "LLC", "L.L.C.", "Ltd", "Ltd.", "Limited",
	"GmbH", "Corp", "Corp.", "Corporation", "Co.",
	"S.A.", "S.A", "AG", "PLC", "Pty", "B.V.", "BV",
	"S.r.l.", "S.R.L.", "SRL", "ApS", "A/S",
}

// looksLikeCompanyName checks if a name appears to be a company rather than an individual
func looksLikeCompanyName(name string) bool {
	upperName := strings.ToUpper(name)
	for _, suffix := range companySuffixes {
		if strings.Contains(upperName, strings.ToUpper(suffix)) {
			return true
		}
	}
	return false
}

// extractCompanyFromCopyright attempts to extract a company name from NSHumanReadableCopyright
// Examples:
//   - "© 2023 CoScreen GmbH. All rights reserved." → "CoScreen GmbH"
//   - "Copyright 2024 Datadog, Inc." → "Datadog, Inc."
//   - "Copyright © 2025 CoScreen. CoScreen is a Datadog company." → "CoScreen"
func extractCompanyFromCopyright(copyright string) string {
	if copyright == "" {
		return ""
	}

	// Remove common copyright prefixes and symbols
	cleaned := copyright
	cleaned = strings.ReplaceAll(cleaned, "©", "")
	cleaned = strings.ReplaceAll(cleaned, "(C)", "")
	cleaned = strings.ReplaceAll(cleaned, "(c)", "")
	cleaned = strings.ReplaceAll(cleaned, "Copyright", "")
	cleaned = strings.ReplaceAll(cleaned, "copyright", "")
	cleaned = strings.TrimSpace(cleaned)

	// Remove leading year or year range (e.g., "2023 CoScreen" or "2019-2025 Munki" → company name)
	yearPattern := regexp.MustCompile(`^\d{4}(-\d{4})?\s+`)
	cleaned = yearPattern.ReplaceAllString(cleaned, "")

	// Try to find a company name by looking for company suffixes
	// Pattern: look for text ending with a company suffix
	for _, suffix := range companySuffixes {
		// Create pattern to find "Word(s) Suffix" pattern
		pattern := regexp.MustCompile(`(?i)([A-Za-z][A-Za-z0-9\s,\-\.&']+\s*` + regexp.QuoteMeta(suffix) + `)`)
		matches := pattern.FindStringSubmatch(cleaned)
		if len(matches) >= 2 {
			result := strings.TrimSpace(matches[1])
			if result != "" {
				return result
			}
		}
	}

	// If no company suffix found, extract the first name/word before common delimiters
	// This handles cases like "CoScreen. CoScreen is a Datadog company." → "CoScreen"
	// Common delimiters: period followed by space, comma, "All rights", "is a", etc.
	delimiters := []string{". ", ", a ", " is a ", " - ", " All rights", " all rights"}
	for _, delim := range delimiters {
		if idx := strings.Index(cleaned, delim); idx > 0 {
			result := strings.TrimSpace(cleaned[:idx])
			// Ensure we got something meaningful (at least 2 chars, not just a year)
			if len(result) >= 2 && !regexp.MustCompile(`^\d+$`).MatchString(result) {
				return result
			}
		}
	}

	// Last resort: take everything before "All rights reserved" or similar
	lowerCleaned := strings.ToLower(cleaned)
	if idx := strings.Index(lowerCleaned, "all rights"); idx > 0 {
		result := strings.TrimSpace(cleaned[:idx])
		// Remove trailing punctuation
		result = strings.TrimRight(result, ".,;:")
		if len(result) >= 2 {
			return result
		}
	}

	return ""
}

// extractPublisherFromBundleID attempts to extract publisher from bundle ID
// Examples:
//   - "com.microsoft.Word" → "Microsoft"
//   - "com.adobe.Photoshop" → "Adobe"
//   - "com.apple.Safari" → "Apple"
func extractPublisherFromBundleID(bundleID string) string {
	// Split by dots and take the second component (company name)
	parts := strings.Split(bundleID, ".")
	if len(parts) >= 2 {
		company := parts[1]
		// Capitalize first letter
		if len(company) > 0 {
			return strings.ToUpper(company[:1]) + company[1:]
		}
	}
	return ""
}

// getPublisherFromPlistData extracts publisher from a pre-parsed plist map (no I/O).
// Priority order:
// 1. NSHumanReadableCopyright (extract company name)
// 2. CFBundleIdentifier (extract from reverse DNS, e.g., com.microsoft.* → "Microsoft")
// 3. CFBundleName (fallback, may contain company name)
func getPublisherFromPlistData(plistData map[string]string) string {
	// Priority 1: NSHumanReadableCopyright
	if copyright, ok := plistData["NSHumanReadableCopyright"]; ok && copyright != "" {
		if publisher := extractCompanyFromCopyright(copyright); publisher != "" {
			return publisher
		}
	}

	// Priority 2: Extract from CFBundleIdentifier (reverse DNS)
	// e.g., "com.microsoft.Word" → "Microsoft"
	if bundleID, ok := plistData["CFBundleIdentifier"]; ok && bundleID != "" {
		if publisher := extractPublisherFromBundleID(bundleID); publisher != "" {
			return publisher
		}
	}

	// Priority 3: Try CFBundleName (may contain company name)
	// This is less reliable but can help for some apps
	if bundleName, ok := plistData["CFBundleName"]; ok && bundleName != "" {
		// Only use if it looks like a company name
		if looksLikeCompanyName(bundleName) {
			return bundleName
		}
	}

	return ""
}

// getPublisherFromInfoPlist extracts publisher from Info.plist at the given bundle path.
// It reads the plist from disk and delegates to getPublisherFromPlistData.
func getPublisherFromInfoPlist(bundlePath string) string {
	infoPlistPath := filepath.Join(bundlePath, "Contents", "Info.plist")
	plistData, err := readPlistFile(infoPlistPath)
	if err != nil {
		return ""
	}
	return getPublisherFromPlistData(plistData)
}

// pkgInfo contains information about a package installation from pkgutil
type pkgInfo struct {
	// PkgID is the package identifier (e.g., "com.microsoft.Word")
	PkgID string
	// Volume is the install volume (e.g., "/")
	Volume string
	// InstallTime is the installation timestamp
	InstallTime string
}

// getPkgInfo queries the macOS package receipt database to find which PKG installed
// a specific file or directory. This uses `pkgutil --file-info` which is the official
// way to link applications to their installer receipts.
//
// Parameters:
//   - path: The path to query (e.g., "/Applications/Numbers.app")
//
// Returns:
//   - *pkgInfo: Package information if the path was installed by a PKG, nil otherwise
//
// Note: Returns nil for apps installed via drag-and-drop (no PKG receipt) or
// Mac App Store apps (receipt stored inside the app bundle, not in pkgutil database).
func getPkgInfo(path string) *pkgInfo {
	// Run pkgutil --file-info to query which package installed this path
	cmd := exec.Command("pkgutil", "--file-info", path)
	output, err := cmd.Output()
	if err != nil {
		// No package owns this path (drag-and-drop install or error)
		return nil
	}

	// Parse the output which looks like:
	// volume: /
	// path: Applications/Numbers.app
	// pkgid: com.apple.pkg.Numbers
	// pkg-version: 14.0
	// install-time: 1654432493
	info := &pkgInfo{}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pkgid: ") {
			info.PkgID = strings.TrimPrefix(line, "pkgid: ")
		} else if strings.HasPrefix(line, "volume: ") {
			info.Volume = strings.TrimPrefix(line, "volume: ")
		} else if strings.HasPrefix(line, "install-time: ") {
			// Convert Unix timestamp to ISO 8601 format for cross-platform consistency
			timestampStr := strings.TrimPrefix(line, "install-time: ")
			if unixTime, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
				info.InstallTime = time.Unix(unixTime, 0).Format(time.RFC3339)
			}
		}
	}

	// Only return if we found a package ID
	if info.PkgID != "" {
		return info
	}
	return nil
}

// entryWithPath pairs an Entry with its bundle path for parallel processing.
// When plistData is non-nil, the worker uses it to compute publisher without reading the file.
type entryWithPath struct {
	entry     *Entry
	path      string
	plistData map[string]string // optional: already-read plist; nil means read from path
}

// populatePublishersParallel gets publisher info for multiple entries in parallel
// Uses a worker pool to limit concurrent operations
func populatePublishersParallel(items []entryWithPath) {
	const maxWorkers = 10 // Limit concurrent operations

	if len(items) == 0 {
		return
	}

	// Create buffered channel with capacity = number of items
	jobs := make(chan *entryWithPath, len(items))
	// Queue items into the channel
	for i := range items {
		jobs <- &items[i]
	}
	close(jobs)

	var wg sync.WaitGroup
	workerCount := maxWorkers
	if len(items) < maxWorkers {
		workerCount = len(items)
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if item.plistData != nil {
					item.entry.Publisher = getPublisherFromPlistData(item.plistData)
				} else {
					item.entry.Publisher = getPublisherFromInfoPlist(item.path)
				}
			}
		}()
	}

	wg.Wait()
}
