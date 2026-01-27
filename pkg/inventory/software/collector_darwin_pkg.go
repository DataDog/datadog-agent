// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// pkgReceiptsCollector collects software from PKG installer receipts
// This collector filters out receipts for applications that are already captured
// by the applicationsCollector (apps in /Applications), to avoid confusing duplicates.
type pkgReceiptsCollector struct{}

// pkgFilesCache holds cached results from pkgutil --files queries
type pkgFilesCache struct {
	mu    sync.RWMutex
	cache map[string][]string // pkgID -> list of file paths
}

// newPkgFilesCache creates a new cache for pkgutil --files results
func newPkgFilesCache() *pkgFilesCache {
	return &pkgFilesCache{
		cache: make(map[string][]string),
	}
}

// get retrieves cached file list for a package, or fetches it if not cached
func (c *pkgFilesCache) get(pkgID string) []string {
	c.mu.RLock()
	if files, ok := c.cache[pkgID]; ok {
		c.mu.RUnlock()
		return files
	}
	c.mu.RUnlock()

	// Not in cache, fetch it
	files := fetchPkgFiles(pkgID)

	c.mu.Lock()
	c.cache[pkgID] = files
	c.mu.Unlock()

	return files
}

// prefetch fetches pkgutil --files for multiple packages in parallel
func (c *pkgFilesCache) prefetch(pkgIDs []string) {
	var wg sync.WaitGroup
	for _, pkgID := range pkgIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c.get(id) // This will fetch and cache if not already cached
		}(pkgID)
	}
	wg.Wait()
}

// fetchPkgFiles runs pkgutil --files and returns the list of files
func fetchPkgFiles(pkgID string) []string {
	cmd := exec.Command("pkgutil", "--files", pkgID)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// pkgInstalledAppFromCache checks if a package installed an application bundle
// using cached file list instead of calling pkgutil again
func pkgInstalledAppFromCache(files []string) bool {
	for _, line := range files {
		// Check if this line represents an .app bundle
		// We look for .app in the path and verify it's a bundle (not just a file with .app in name)
		if strings.Contains(line, ".app") {
			// Get the first path component to check if it's the app bundle itself
			// or if it's inside an Applications directory
			parts := strings.Split(line, "/")

			// Case 1: Direct app bundle (InstallPrefixPath = "Applications")
			// e.g., "Google Chrome.app" or "Google Chrome.app/Contents"
			if strings.HasSuffix(parts[0], ".app") {
				return true
			}

			// Case 2: App inside Applications folder (InstallPrefixPath = "/")
			// e.g., "Applications/Numbers.app" or "Applications/Numbers.app/Contents"
			if len(parts) >= 2 && parts[0] == "Applications" && strings.HasSuffix(parts[1], ".app") {
				return true
			}
		}
	}

	return false
}

// isLikelyFile checks if a path looks like a file (not a directory).
// Files typically have extensions or are in known executable locations.
func isLikelyFile(path string) bool {
	// Common file extensions
	fileExtensions := []string{
		".so", ".dylib", ".a", ".o", // Libraries
		".py", ".pyc", ".pyo", ".pyd", // Python
		".rb", ".pl", ".sh", ".bash", // Scripts
		".json", ".yaml", ".yml", ".xml", ".plist", // Config
		".txt", ".md", ".rst", ".html", ".css", ".js", // Text/Web
		".png", ".jpg", ".jpeg", ".gif", ".ico", ".icns", // Images
		".app", ".framework", ".bundle", ".kext", // macOS bundles
		".pkg", ".dmg", ".zip", ".tar", ".gz", // Archives
		".conf", ".cfg", ".ini", ".log", // Config/logs
		".h", ".c", ".cpp", ".m", ".swift", // Source
		".strings", ".nib", ".xib", ".storyboard", // macOS resources
	}

	// Check for file extension
	for _, ext := range fileExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	// Check if it's in a bin directory (executables often have no extension)
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "bin" && i < len(parts)-1 {
			// The item after "bin" is likely an executable
			return true
		}
	}

	// Check for common executable names without extensions
	lastPart := parts[len(parts)-1]
	if !strings.Contains(lastPart, ".") && len(lastPart) > 0 {
		// Files in certain directories are likely files, not directories
		for _, part := range parts {
			if part == "bin" || part == "lib" || part == "share" || part == "include" {
				return true
			}
		}
	}

	return false
}

// getPkgTopLevelPathsFromCache extracts top-level directories from cached file list
func getPkgTopLevelPathsFromCache(files []string, prefixPath string) []string {
	// Normalize prefix path for building absolute paths
	var basePrefix string
	if prefixPath == "" || prefixPath == "/" {
		basePrefix = ""
	} else if strings.HasPrefix(prefixPath, "/") {
		basePrefix = prefixPath
	} else {
		basePrefix = "/" + prefixPath
	}

	// Collect file parent directories at appropriate depth
	dirSet := make(map[string]bool)

	for _, line := range files {
		// Only process files (items with extensions or in known file locations)
		if !isLikelyFile(line) {
			continue
		}

		// Get path components
		parts := strings.Split(line, "/")
		if len(parts) == 0 {
			continue
		}

		// Determine the meaningful top-level directory based on path structure
		// We want to capture the "application directory" level, not every nested dir
		var topLevelDir string

		switch parts[0] {
		case "usr":
			// For /usr paths, capture at the 3rd level (e.g., /usr/local/bin, /usr/local/ykman)
			if len(parts) >= 3 {
				topLevelDir = "/" + parts[0] + "/" + parts[1] + "/" + parts[2]
			}
		case "Library":
			// For /Library, capture at 2nd level (e.g., /Library/LaunchDaemons)
			if len(parts) >= 2 {
				topLevelDir = "/" + parts[0] + "/" + parts[1]
			}
		case "opt":
			// For /opt, capture the application directory (e.g., /opt/datadog-agent)
			if len(parts) >= 2 {
				topLevelDir = "/" + parts[0] + "/" + parts[1]
			}
		case "Applications":
			// For /Applications, capture the app bundle (e.g., /Applications/Chrome.app)
			if len(parts) >= 2 {
				topLevelDir = "/" + parts[0] + "/" + parts[1]
			}
		case "System", "private", "var":
			// For system paths, capture at 3rd level
			if len(parts) >= 3 {
				topLevelDir = "/" + parts[0] + "/" + parts[1] + "/" + parts[2]
			} else if len(parts) >= 2 {
				topLevelDir = "/" + parts[0] + "/" + parts[1]
			}
		default:
			// For paths with a prefix (e.g., "Applications" prefix), combine with first component
			if basePrefix != "" && basePrefix != "/" {
				topLevelDir = basePrefix + "/" + parts[0]
			} else if len(parts) >= 1 {
				topLevelDir = "/" + parts[0]
			}
		}

		// Clean up and add to set
		if topLevelDir != "" && topLevelDir != "/" {
			topLevelDir = strings.ReplaceAll(topLevelDir, "//", "/")
			dirSet[topLevelDir] = true
		}
	}

	// Convert map to sorted slice
	paths := make([]string, 0, len(dirSet))
	for path := range dirSet {
		paths = append(paths, path)
	}

	// Sort for consistent output
	if len(paths) > 1 {
		for i := 0; i < len(paths)-1; i++ {
			for j := i + 1; j < len(paths); j++ {
				if paths[i] > paths[j] {
					paths[i], paths[j] = paths[j], paths[i]
				}
			}
		}
	}

	return paths
}

// pkgReceiptInfo holds parsed info from a PKG receipt plist
type pkgReceiptInfo struct {
	packageID   string
	version     string
	installDate string
	prefixPath  string
}

// Collect reads PKG installer receipts from /var/db/receipts
// It filters out:
//   - Mac App Store receipts (ending in _MASReceipt) - these are handled by applicationsCollector
//   - Packages that installed .app bundles to /Applications - already captured by applicationsCollector
//
// This ensures PKG receipts only show non-application installations like:
//   - System components and frameworks
//   - Command-line tools
//   - Drivers and kernel extensions
//   - Libraries and shared resources
func (c *pkgReceiptsCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	receiptsDir := "/var/db/receipts"

	dirEntries, err := os.ReadDir(receiptsDir)
	if err != nil {
		// Not an error if receipts directory doesn't exist
		if os.IsNotExist(err) {
			return entries, warnings, nil
		}
		return nil, nil, err
	}

	// First pass: Read all receipt plists and collect package IDs
	var receipts []pkgReceiptInfo
	var pkgIDsToFetch []string

	for _, dirEntry := range dirEntries {
		if !strings.HasSuffix(dirEntry.Name(), ".plist") {
			continue
		}

		receiptPath := filepath.Join(receiptsDir, dirEntry.Name())
		plistData, err := readPlistFile(receiptPath)
		if err != nil {
			warnings = append(warnings, warnf("failed to read receipt %s: %v", dirEntry.Name(), err))
			continue
		}

		// Get package identifier as both display name and product code
		packageID := plistData["PackageIdentifier"]
		if packageID == "" {
			continue
		}

		// Skip Mac App Store receipts - these correspond to MAS apps which are
		// already captured by applicationsCollector with richer metadata
		if strings.HasSuffix(packageID, "_MASReceipt") {
			continue
		}

		// Get install prefix path from receipt
		prefixPath := plistData["InstallPrefixPath"]
		if prefixPath == "" {
			prefixPath = plistData["InstallLocation"]
		}

		receipts = append(receipts, pkgReceiptInfo{
			packageID:   packageID,
			version:     plistData["PackageVersion"],
			installDate: plistData["InstallDate"],
			prefixPath:  prefixPath,
		})
		pkgIDsToFetch = append(pkgIDsToFetch, packageID)
	}

	// Prefetch all pkgutil --files results in parallel
	cache := newPkgFilesCache()
	cache.prefetch(pkgIDsToFetch)

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Second pass: Process receipts using cached data
	for _, receipt := range receipts {
		files := cache.get(receipt.packageID)

		// Skip packages that installed applications to /Applications
		// These are already captured by applicationsCollector
		if pkgInstalledAppFromCache(files) {
			continue
		}

		// Determine install_path for backward compatibility
		var installPath string
		if receipt.prefixPath != "" && receipt.prefixPath != "/" {
			if !strings.HasPrefix(receipt.prefixPath, "/") {
				installPath = "/" + receipt.prefixPath
			} else {
				installPath = receipt.prefixPath
			}
		} else {
			installPath = "N/A"
		}

		// Get top-level installation directories from cached file list
		installPaths := getPkgTopLevelPathsFromCache(files, receipt.prefixPath)

		// Filter out generic system directories
		filteredPaths := make([]string, 0, len(installPaths))
		for _, p := range installPaths {
			if p == "/etc" || p == "/var" || p == "/tmp" || p == "/System" {
				continue
			}
			filteredPaths = append(filteredPaths, p)
		}
		installPaths = filteredPaths

		// Determine which path field(s) to include
		if installPath != "N/A" && len(installPaths) > 0 {
			hasPathsOutside := false
			installPathWithSlash := installPath + "/"
			for _, p := range installPaths {
				if !strings.HasPrefix(p, installPathWithSlash) && p != installPath {
					hasPathsOutside = true
					break
				}
			}
			if !hasPathsOutside {
				installPaths = nil
			}
		} else if installPath == "N/A" && len(installPaths) > 0 {
			if len(installPaths) == 1 {
				installPath = installPaths[0]
				installPaths = nil
			} else {
				installPath = ""
			}
		}

		// Check if the installation location still exists
		status := statusInstalled
		var brokenReason string
		if installPath != "" && installPath != "N/A" {
			if _, err := os.Stat(installPath); os.IsNotExist(err) {
				status = statusBroken
				brokenReason = fmt.Sprintf("install path not found: %s", installPath)
			}
		} else if len(installPaths) > 0 {
			for _, p := range installPaths {
				if _, err := os.Stat(p); os.IsNotExist(err) {
					status = statusBroken
					brokenReason = fmt.Sprintf("install path not found: %s", p)
					break
				}
			}
		}

		entry := &Entry{
			DisplayName:  receipt.packageID,
			Version:      receipt.version,
			InstallDate:  receipt.installDate,
			Source:       softwareTypePkg,
			ProductCode:  receipt.packageID,
			Status:       status,
			BrokenReason: brokenReason,
			Is64Bit:      is64Bit,
			InstallPath:  installPath,
			InstallPaths: installPaths,
		}

		entries = append(entries, entry)
	}

	return entries, warnings, nil
}
