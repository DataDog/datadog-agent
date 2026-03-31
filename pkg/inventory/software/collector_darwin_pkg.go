// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// pkgReceiptsCollector collects software from PKG installer receipts
// This collector filters out receipts for applications that are already captured
// by the applicationsCollector (apps in /Applications), to avoid confusing duplicates.
type pkgReceiptsCollector struct{}

// pkgSummary stores compact derived facts from pkgutil --files output.
// It intentionally avoids retaining full file lists to reduce memory usage.
type pkgSummary struct {
	// HasApplicationsApp is true when pkg payload contains an app bundle under /Applications.
	HasApplicationsApp bool
	// HasNonAppPayload is true when pkg payload includes likely files outside /Applications app bundles.
	HasNonAppPayload bool
	// TopLevelPaths stores deduplicated top-level install directories derived from pkg payload file paths.
	TopLevelPaths []string
}

// pkgFilesCacheEntry holds a cached package summary with its timestamp for TTL checking.
type pkgFilesCacheEntry struct {
	Summary   pkgSummary
	Timestamp time.Time
}

// pkgFilesCache holds cached results from pkgutil --files queries
type pkgFilesCache struct {
	mu             sync.RWMutex
	cache          map[string]*pkgFilesCacheEntry
	ttl            time.Duration
	maxEntries     int
	fetchSummaryFn func(pkgID, prefixPath string) pkgSummary
	sfGroup        singleflight.Group
}

// Default TTL for pkgutil --files cache entries
const (
	defaultPkgFilesCacheTTL      = 1 * time.Hour
	defaultPkgFilesCacheMaxItems = 512
	pkgutilFilesCommandTimeout   = 30 * time.Second
	pkgutilScannerMaxTokenSize   = 2 * 1024 * 1024
	maxTopLevelPathsPerPackage   = 128
)

// Global cache instance for pkgutil --files results
var (
	globalPkgFilesCache *pkgFilesCache
	globalCacheOnce     sync.Once
)

// getGlobalPkgFilesCache returns the global singleton cache instance
// The cache persists across collection runs within the same process lifetime
func getGlobalPkgFilesCache() *pkgFilesCache {
	globalCacheOnce.Do(func() {
		globalPkgFilesCache = &pkgFilesCache{
			cache:      make(map[string]*pkgFilesCacheEntry),
			ttl:        defaultPkgFilesCacheTTL,
			maxEntries: defaultPkgFilesCacheMaxItems,
		}
	})
	return globalPkgFilesCache
}

// makePkgCacheKey builds a stable cache key from package ID and normalized install prefix.
func makePkgCacheKey(pkgID, prefixPath string) string {
	normalizedPrefix := strings.TrimSpace(prefixPath)
	if normalizedPrefix == "/" {
		return pkgID + "|/"
	}
	normalizedPrefix = strings.TrimSuffix(normalizedPrefix, "/")
	return pkgID + "|" + normalizedPrefix
}

// get retrieves a cached package summary, or fetches it if not cached or expired.
func (c *pkgFilesCache) get(pkgID, prefixPath string) pkgSummary {
	key := makePkgCacheKey(pkgID, prefixPath)
	now := time.Now()

	// Check cache with read lock
	c.mu.RLock()
	entry, ok := c.cache[key]
	if ok && entry != nil {
		// Check if entry is still valid (not expired)
		age := now.Sub(entry.Timestamp)
		if age < c.ttl {
			// Cache hit - entry is valid
			summary := entry.Summary
			c.mu.RUnlock()
			return summary
		}
		// Entry exists but is expired - will fetch new data below
	}
	c.mu.RUnlock()

	// Not in cache or expired: use singleflight so only one goroutine runs pkgutil per key.
	v, err, _ := c.sfGroup.Do(key, func() (interface{}, error) {
		fetchSummary := c.fetchSummaryFn
		if fetchSummary == nil {
			fetchSummary = streamPkgSummary
		}
		summary := fetchSummary(pkgID, prefixPath)

		c.mu.Lock()
		c.deleteExpiredLocked(time.Now())
		if c.maxEntries > 0 {
			if _, exists := c.cache[key]; !exists && len(c.cache) >= c.maxEntries {
				c.evictOldestLocked()
			}
		}
		c.cache[key] = &pkgFilesCacheEntry{
			Summary:   summary,
			Timestamp: time.Now(),
		}
		c.mu.Unlock()

		return summary, nil
	})
	if err != nil {
		return pkgSummary{}
	}
	return v.(pkgSummary)
}

// deleteExpiredLocked removes stale cache entries older than the configured TTL.
func (c *pkgFilesCache) deleteExpiredLocked(now time.Time) {
	for key, entry := range c.cache {
		if entry == nil || now.Sub(entry.Timestamp) >= c.ttl {
			delete(c.cache, key)
		}
	}
}

// evictOldestLocked removes the oldest cache entry to honor the maxEntries limit.
func (c *pkgFilesCache) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for key, entry := range c.cache {
		if entry == nil {
			oldestKey = key
			first = false
			break
		}
		if first || entry.Timestamp.Before(oldest) {
			oldest = entry.Timestamp
			oldestKey = key
			first = false
		}
	}
	if !first {
		delete(c.cache, oldestKey)
	}
}

// prefetch fetches pkgutil --files for multiple packages in parallel
// Uses a worker pool to limit concurrent pkgutil processes
func (c *pkgFilesCache) prefetch(receipts []pkgReceiptInfo) {
	const maxWorkers = 10 // Limit concurrent pkgutil processes

	if len(receipts) == 0 {
		return
	}

	// Create a channel for work items
	jobs := make(chan pkgReceiptInfo, len(receipts))
	for _, receipt := range receipts {
		jobs <- receipt
	}
	close(jobs)

	// Start worker pool
	var wg sync.WaitGroup
	workerCount := maxWorkers
	if len(receipts) < maxWorkers {
		workerCount = len(receipts)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for receipt := range jobs {
				c.get(receipt.packageID, receipt.prefixPath) // This will fetch and cache if not already cached
			}
		}()
	}

	wg.Wait()
}

// isApplicationsAppPath reports whether a pkg file path belongs to an app bundle in Applications.
func isApplicationsAppPath(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return false
	}
	if strings.HasSuffix(parts[0], ".app") {
		return true
	}
	return len(parts) >= 2 && parts[0] == "Applications" && strings.HasSuffix(parts[1], ".app")
}

// topLevelPathFromLine derives a representative top-level install path from a pkg payload line.
func topLevelPathFromLine(line, prefixPath string) string {
	parts := strings.Split(line, "/")
	if len(parts) == 0 {
		return ""
	}

	// Normalize prefix path for building absolute paths
	var basePrefix string
	if prefixPath == "" || prefixPath == "/" {
		basePrefix = ""
	} else if strings.HasPrefix(prefixPath, "/") {
		basePrefix = prefixPath
	} else {
		basePrefix = "/" + prefixPath
	}

	var topLevelDir string
	switch parts[0] {
	case "usr":
		if len(parts) >= 3 {
			topLevelDir = "/" + parts[0] + "/" + parts[1] + "/" + parts[2]
		}
	case "Library":
		if len(parts) >= 2 {
			topLevelDir = "/" + parts[0] + "/" + parts[1]
		}
	case "opt":
		if len(parts) >= 2 {
			topLevelDir = "/" + parts[0] + "/" + parts[1]
		}
	case "Applications":
		if len(parts) >= 2 {
			topLevelDir = "/" + parts[0] + "/" + parts[1]
		}
	case "System", "private", "var":
		if len(parts) >= 3 {
			topLevelDir = "/" + parts[0] + "/" + parts[1] + "/" + parts[2]
		} else if len(parts) >= 2 {
			topLevelDir = "/" + parts[0] + "/" + parts[1]
		}
	default:
		if basePrefix != "" && basePrefix != "/" {
			topLevelDir = basePrefix + "/" + parts[0]
		} else if len(parts) >= 1 {
			topLevelDir = "/" + parts[0]
		}
	}

	if topLevelDir == "" || topLevelDir == "/" {
		return ""
	}
	return strings.ReplaceAll(topLevelDir, "//", "/")
}

// updatePkgSummaryFromLine updates summary flags and top-level path set from one pkg payload line.
func updatePkgSummaryFromLine(summary *pkgSummary, topLevelSet map[string]struct{}, line, prefixPath string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	appPath := isApplicationsAppPath(line)
	if appPath {
		summary.HasApplicationsApp = true
	}

	// We only track payload based on likely files, not directory marker lines.
	if !isLikelyFile(line) {
		return
	}

	if !appPath {
		summary.HasNonAppPayload = true
	}

	topLevelDir := topLevelPathFromLine(line, prefixPath)
	if topLevelDir == "" {
		return
	}
	if _, exists := topLevelSet[topLevelDir]; exists || len(topLevelSet) < maxTopLevelPathsPerPackage {
		topLevelSet[topLevelDir] = struct{}{}
	}
}

// buildPkgSummaryFromLines builds a compact package summary from pkgutil --files output lines.
func buildPkgSummaryFromLines(lines []string, prefixPath string) pkgSummary {
	summary := pkgSummary{}
	topLevelSet := make(map[string]struct{})
	for _, line := range lines {
		updatePkgSummaryFromLine(&summary, topLevelSet, line, prefixPath)
	}
	summary.TopLevelPaths = sortedPathsFromSet(topLevelSet)
	return summary
}

// sortedPathsFromSet converts a path set into a lexicographically sorted slice.
func sortedPathsFromSet(paths map[string]struct{}) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// streamPkgSummary runs pkgutil --files and summarizes package contents in a single pass.
func streamPkgSummary(pkgID, prefixPath string) pkgSummary {
	ctx, cancel := context.WithTimeout(context.Background(), pkgutilFilesCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pkgutil", "--files", pkgID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return pkgSummary{}
	}
	if err := cmd.Start(); err != nil {
		return pkgSummary{}
	}

	summary := pkgSummary{}
	topLevelSet := make(map[string]struct{})
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), pkgutilScannerMaxTokenSize)
	for scanner.Scan() {
		updatePkgSummaryFromLine(&summary, topLevelSet, scanner.Text(), prefixPath)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		_ = cmd.Wait()
		return pkgSummary{}
	}
	if err := cmd.Wait(); err != nil {
		// Preserve old behavior on pkgutil failure: keep package with minimal metadata
		// by returning an empty summary (which does not trigger app-only skip).
		return pkgSummary{}
	}

	summary.TopLevelPaths = sortedPathsFromSet(topLevelSet)
	return summary
}

// shouldSkipPkgFromSummary applies baseline pkg suppression semantics for app-backed software.
func shouldSkipPkgFromSummary(summary pkgSummary) bool {
	// If a package installs any app bundle in /Applications,
	// prefer the app representation and skip the package representation.
	return summary.HasApplicationsApp
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

// filterGenericSystemPaths removes overly generic install roots from summarized path output.
func filterGenericSystemPaths(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "/etc" || path == "/var" || path == "/tmp" || path == "/System" {
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
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
	}

	// Prefetch all pkgutil --files summaries in parallel
	// Use global cache that persists across collection runs
	cache := getGlobalPkgFilesCache()
	cache.prefetch(receipts)

	// Determine architecture
	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Second pass: Process receipts using cached data
	for _, receipt := range receipts {
		summary := cache.get(receipt.packageID, receipt.prefixPath)

		// Skip packages with Applications representation.
		if shouldSkipPkgFromSummary(summary) {
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

		// Use summary top-level installation directories and filter generic system directories
		installPaths := filterGenericSystemPaths(summary.TopLevelPaths)

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
				brokenReason = "install path not found: " + installPath
			}
		} else if len(installPaths) > 0 {
			for _, p := range installPaths {
				if _, err := os.Stat(p); os.IsNotExist(err) {
					status = statusBroken
					brokenReason = "install path not found: " + p
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
