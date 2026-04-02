// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

// pkgReceiptsCollector collects software from PKG installer receipts
// This collector filters out receipts for applications that are already captured
// by the applicationsCollector (apps in /Applications), to avoid confusing duplicates.
type pkgReceiptsCollector struct{}

// pkgSummary stores compact derived facts from lsbom directory listing output.
// It intentionally avoids retaining full file lists to reduce memory usage.
type pkgSummary struct {
	// HasApplicationsApp is true when pkg payload contains an app bundle under /Applications.
	HasApplicationsApp bool
	// HasNonAppPayload is true when pkg payload includes directories outside /Applications app bundles.
	HasNonAppPayload bool
	// TopLevelPaths stores deduplicated top-level install directories derived from pkg payload directory paths.
	TopLevelPaths []string
}

const (
	defaultBomCacheTTL        = 1 * time.Hour
	defaultBomCacheMaxEntries = 512
	lsbomBatchTimeout         = 60 * time.Second
	lsbomSingleTimeout        = 30 * time.Second
	lsbomScannerMaxTokenSize  = 2 * 1024 * 1024
	maxTopLevelPathsPerPkg    = 128
	bomDelimiterPrefix        = "===BOM:"
	bomDelimiterSuffix        = "==="
)

// bomCacheEntry stores cached raw directory lines from lsbom for a single BOM file.
type bomCacheEntry struct {
	Lines     []string
	Timestamp time.Time
}

// bomCache caches raw lsbom -sd output lines keyed by BOM file path.
// The summary (pkgSummary) is derived per-receipt from cached lines + prefixPath,
// so the same BOM data can serve receipts with different install prefixes.
type bomCache struct {
	mu         sync.Mutex
	entries    map[string]*bomCacheEntry
	ttl        time.Duration
	maxEntries int
}

var (
	globalBomCache     *bomCache
	globalBomCacheOnce sync.Once
)

func getGlobalBomCache() *bomCache {
	globalBomCacheOnce.Do(func() {
		globalBomCache = &bomCache{
			entries:    make(map[string]*bomCacheEntry),
			ttl:        defaultBomCacheTTL,
			maxEntries: defaultBomCacheMaxEntries,
		}
	})
	return globalBomCache
}

// getBomLines returns cached lsbom lines for the given BOM paths.
// Uncached or expired entries are fetched in a single batched shell subprocess.
func (c *bomCache) getBomLines(bomPaths []string) map[string][]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	result := make(map[string][]string, len(bomPaths))
	var uncached []string

	for _, bp := range bomPaths {
		if entry, ok := c.entries[bp]; ok && now.Sub(entry.Timestamp) < c.ttl {
			result[bp] = entry.Lines
		} else {
			uncached = append(uncached, bp)
		}
	}

	if len(uncached) == 0 {
		return result
	}

	fetched := batchLsbom(uncached)

	// Evict expired entries before inserting new ones
	for key, entry := range c.entries {
		if now.Sub(entry.Timestamp) >= c.ttl {
			delete(c.entries, key)
		}
	}

	for _, bp := range uncached {
		lines := fetched[bp]
		if lines == nil {
			lines = []string{}
		}

		// Evict oldest if at capacity
		if len(c.entries) >= c.maxEntries {
			c.evictOldestLocked()
		}

		c.entries[bp] = &bomCacheEntry{Lines: lines, Timestamp: now}
		result[bp] = lines
	}

	return result
}

func (c *bomCache) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for key, entry := range c.entries {
		if first || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
			first = false
		}
	}
	if !first {
		delete(c.entries, oldestKey)
	}
}

// shellUnsafeChars contains characters that could enable command injection inside
// single-quoted shell strings. BOM paths containing any of these are handled via
// a dedicated exec.Command instead of the batched shell script.
const shellUnsafeChars = "'\"`$;|&(){}!\\#~<>?\n\r\x00"

// isSafeForShell reports whether a path can be safely embedded in a single-quoted
// shell argument without risk of command injection.
func isSafeForShell(path string) bool {
	return !strings.ContainsAny(path, shellUnsafeChars)
}

// singleLsbom runs lsbom -sd for one BOM file using exec.Command directly,
// bypassing the shell. This is injection-safe for any filename.
func singleLsbom(bomPath string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), lsbomSingleTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lsbom", "-sd", bomPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return []string{}
	}
	if err := cmd.Start(); err != nil {
		return []string{}
	}

	var lines []string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), lsbomScannerMaxTokenSize)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := cmd.Wait(); err != nil {
		log.Warnf("lsbom failed for %s: %v", bomPath, err)
	}
	return lines
}

// batchLsbomShell runs a single shell subprocess that invokes lsbom -sd for multiple
// BOM files, producing delimited output. Only safe paths should be passed here.
func batchLsbomShell(bomPaths []string) map[string][]string {
	if len(bomPaths) == 0 {
		return nil
	}

	var script strings.Builder
	for _, bp := range bomPaths {
		fmt.Fprintf(&script, "printf '===BOM:%s===\\n'; lsbom -sd '%s' 2>/dev/null; ", bp, bp)
	}

	ctx, cancel := context.WithTimeout(context.Background(), lsbomBatchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", script.String())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}

	result := make(map[string][]string, len(bomPaths))
	var currentBom string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), lsbomScannerMaxTokenSize)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, bomDelimiterPrefix) && strings.HasSuffix(line, bomDelimiterSuffix) {
			currentBom = line[len(bomDelimiterPrefix) : len(line)-len(bomDelimiterSuffix)]
			if result[currentBom] == nil {
				result[currentBom] = []string{}
			}
			continue
		}
		if currentBom != "" {
			result[currentBom] = append(result[currentBom], line)
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Warnf("batched lsbom shell failed: %v", err)
	}
	return result
}

// batchLsbom fetches lsbom -sd output for all given BOM paths.
// Safe paths are batched into a single shell subprocess for performance.
// Paths with shell metacharacters are handled via individual exec.Command calls.
func batchLsbom(bomPaths []string) map[string][]string {
	if len(bomPaths) == 0 {
		return nil
	}

	var safePaths, unsafePaths []string
	for _, bp := range bomPaths {
		if isSafeForShell(bp) {
			safePaths = append(safePaths, bp)
		} else {
			unsafePaths = append(unsafePaths, bp)
		}
	}

	result := batchLsbomShell(safePaths)
	if result == nil {
		result = make(map[string][]string, len(unsafePaths))
	}
	for _, bp := range unsafePaths {
		result[bp] = singleLsbom(bp)
	}
	return result
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

// updatePkgSummaryFromLine updates summary flags and top-level path set from one directory line.
// Input lines come from lsbom -sd, so every line is a directory path prefixed with "./".
func updatePkgSummaryFromLine(summary *pkgSummary, topLevelSet map[string]struct{}, line, prefixPath string) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "./")
	if line == "" || line == "." {
		return
	}

	appPath := isApplicationsAppPath(line)
	if appPath {
		summary.HasApplicationsApp = true
	} else {
		summary.HasNonAppPayload = true
	}

	topLevelDir := topLevelPathFromLine(line, prefixPath)
	if topLevelDir == "" {
		return
	}
	if _, exists := topLevelSet[topLevelDir]; exists || len(topLevelSet) < maxTopLevelPathsPerPkg {
		topLevelSet[topLevelDir] = struct{}{}
	}
}

// buildPkgSummaryFromLines builds a compact package summary from lsbom -sd output lines.
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

// shouldSkipPkgFromSummary applies baseline pkg suppression semantics for app-backed software.
func shouldSkipPkgFromSummary(summary pkgSummary) bool {
	return summary.HasApplicationsApp
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
	bomPath     string
}

// buildEntryFromReceipt builds a software entry for one receipt using a pre-computed summary.
// Returns nil when the receipt should be skipped by representation rules.
func buildEntryFromReceipt(receipt pkgReceiptInfo, summary pkgSummary, is64Bit bool) *Entry {
	if shouldSkipPkgFromSummary(summary) {
		return nil
	}

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

	installPaths := filterGenericSystemPaths(summary.TopLevelPaths)

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

	return &Entry{
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
}

// appToPkgIndex maps absolute app paths (e.g., "/Applications/zoom.us.app") to the
// package identifier that installed them, derived from BOM data. This replaces the
// expensive per-app `pkgutil --file-info` subprocess calls in applicationsCollector.
type appToPkgIndex struct {
	mu    sync.Mutex
	index map[string]string // appPath → pkgID
	built bool
}

var (
	globalAppToPkgIndex     *appToPkgIndex
	globalAppToPkgIndexOnce sync.Once
)

func getGlobalAppToPkgIndex() *appToPkgIndex {
	globalAppToPkgIndexOnce.Do(func() {
		globalAppToPkgIndex = &appToPkgIndex{}
	})
	return globalAppToPkgIndex
}

// lookupPkgForApp returns the package ID that installed the given app path, or "" if unknown.
// On first call, it reads all PKG receipts and BOM data to build the reverse index.
func (idx *appToPkgIndex) lookupPkgForApp(appPath string) string {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.built {
		idx.index = buildAppToPkgMap()
		idx.built = true
	}
	return idx.index[appPath]
}

// buildAppToPkgMap reads all PKG receipts and their BOM data (via the global cache)
// to build a mapping from absolute app paths to package IDs.
func buildAppToPkgMap() map[string]string {
	receiptsDir := "/var/db/receipts"
	dirEntries, err := os.ReadDir(receiptsDir)
	if err != nil {
		return nil
	}

	type receiptBom struct {
		pkgID      string
		prefixPath string
		bomPath    string
	}

	var items []receiptBom
	bomPathSet := make(map[string]bool)

	for _, de := range dirEntries {
		if !strings.HasSuffix(de.Name(), ".plist") {
			continue
		}
		plistData, err := readPlistFile(filepath.Join(receiptsDir, de.Name()))
		if err != nil {
			continue
		}
		pkgID := plistData["PackageIdentifier"]
		if pkgID == "" {
			continue
		}
		prefixPath := plistData["InstallPrefixPath"]
		if prefixPath == "" {
			prefixPath = plistData["InstallLocation"]
		}
		bomPath := filepath.Join(receiptsDir, strings.TrimSuffix(de.Name(), ".plist")+".bom")
		items = append(items, receiptBom{pkgID: pkgID, prefixPath: prefixPath, bomPath: bomPath})
		bomPathSet[bomPath] = true
	}

	bomPaths := make([]string, 0, len(bomPathSet))
	for bp := range bomPathSet {
		bomPaths = append(bomPaths, bp)
	}

	cache := getGlobalBomCache()
	bomLines := cache.getBomLines(bomPaths)

	result := make(map[string]string)
	for _, item := range items {
		lines := bomLines[item.bomPath]
		prefix := item.prefixPath
		if prefix == "" || prefix == "/" {
			prefix = ""
		} else if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}

		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "./")
			if line == "" || line == "." {
				continue
			}

			// Build absolute path and check if it's an .app in /Applications
			var absPath string
			if prefix == "" {
				absPath = "/" + line
			} else {
				absPath = prefix + "/" + line
			}

			if !strings.HasSuffix(absPath, ".app") {
				continue
			}
			// Only index top-level .app bundles (not nested .app inside other .app)
			dir := filepath.Dir(absPath)
			if dir == "/Applications" || strings.HasPrefix(dir, "/Applications/") ||
				strings.HasPrefix(dir, "/Users/") {
				result[absPath] = item.pkgID
			}
		}
	}
	return result
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
		if os.IsNotExist(err) {
			return entries, warnings, nil
		}
		return nil, nil, err
	}

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

		packageID := plistData["PackageIdentifier"]
		if packageID == "" {
			continue
		}

		if strings.HasSuffix(packageID, "_MASReceipt") {
			continue
		}

		prefixPath := plistData["InstallPrefixPath"]
		if prefixPath == "" {
			prefixPath = plistData["InstallLocation"]
		}

		bomPath := filepath.Join(receiptsDir, strings.TrimSuffix(dirEntry.Name(), ".plist")+".bom")

		receipts = append(receipts, pkgReceiptInfo{
			packageID:   packageID,
			version:     plistData["PackageVersion"],
			installDate: plistData["InstallDate"],
			prefixPath:  prefixPath,
			bomPath:     bomPath,
		})
	}

	is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

	// Collect unique BOM paths
	bomPaths := make([]string, 0, len(receipts))
	seen := make(map[string]bool, len(receipts))
	for _, r := range receipts {
		if !seen[r.bomPath] {
			bomPaths = append(bomPaths, r.bomPath)
			seen[r.bomPath] = true
		}
	}

	// Fetch all BOM data in one batch (cache hit = 0 subprocesses, miss = 1 subprocess)
	cache := getGlobalBomCache()
	bomLines := cache.getBomLines(bomPaths)

	for _, receipt := range receipts {
		lines := bomLines[receipt.bomPath]
		summary := buildPkgSummaryFromLines(lines, receipt.prefixPath)
		if entry := buildEntryFromReceipt(receipt, summary, is64Bit); entry != nil {
			entries = append(entries, entry)
		}
	}

	return entries, warnings, nil
}
