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
	"strings"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

// pkgReceiptsCollector collects software from PKG installer receipts.
// This collector filters out receipts for applications that are already captured
// by the applicationsCollector (apps in /Applications), to avoid confusing duplicates.
type pkgReceiptsCollector struct{}

const (
	defaultBomCacheTTL          = 1 * time.Hour
	defaultBomCacheMaxEntries   = 512
	lsbomSingleTimeout          = 30 * time.Second
	lsbomScannerMaxTokenSize    = 2 * 1024 * 1024
	enableBomDigestCacheDefault = true
)

// bomCacheEntry stores cached compact BOM digest data for a single BOM file.
type bomCacheEntry struct {
	Digest    bomDigest
	Timestamp time.Time
}

// bomCache caches compact BOM digests keyed by BOM file path.
type bomCache struct {
	mu         sync.Mutex
	entries    map[string]*bomCacheEntry
	ttl        time.Duration
	maxEntries int
}

type bomFetchOutcome struct {
	Digest    bomDigest
	Cacheable bool
}

var (
	globalBomCache       *bomCache
	globalBomCacheOnce   sync.Once
	batchLsbomFetcher    = batchLsbom                  // test seam for hermetic cache tests
	enableBomDigestCache = enableBomDigestCacheDefault // Set false for cold-cache experiments.
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

// getBomDigests returns cached compact BOM digests for the given BOM paths.
// Uncached or expired entries are fetched by running lsbom per BOM file.
func (c *bomCache) getBomDigests(bomPaths []string) map[string]bomDigest {
	if !enableBomDigestCache {
		fetched := batchLsbomFetcher(bomPaths)
		result := make(map[string]bomDigest, len(bomPaths))
		for _, bp := range bomPaths {
			if outcome, ok := fetched[bp]; ok {
				result[bp] = outcome.Digest
			} else {
				result[bp] = bomDigest{}
			}
		}
		return result
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	result := make(map[string]bomDigest, len(bomPaths))
	var uncached []string

	for _, bp := range bomPaths {
		if entry, ok := c.entries[bp]; ok && now.Sub(entry.Timestamp) < c.ttl {
			result[bp] = entry.Digest
		} else {
			uncached = append(uncached, bp)
		}
	}

	if len(uncached) == 0 {
		return result
	}

	fetched := batchLsbomFetcher(uncached)

	// Evict expired entries before inserting new ones.
	for key, entry := range c.entries {
		if now.Sub(entry.Timestamp) >= c.ttl {
			delete(c.entries, key)
		}
	}

	for _, bp := range uncached {
		outcome, ok := fetched[bp]
		if ok {
			result[bp] = outcome.Digest
		} else {
			result[bp] = bomDigest{}
		}
		if !ok || !outcome.Cacheable {
			continue
		}

		// Evict oldest if at capacity.
		if len(c.entries) >= c.maxEntries {
			c.evictOldestLocked()
		}

		c.entries[bp] = &bomCacheEntry{Digest: outcome.Digest, Timestamp: now}
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

// singleLsbom runs lsbom -sd for one BOM file and streams output directly into
// digest accumulation. It bypasses the shell and is injection-safe for any name.
func singleLsbom(bomPath string) bomFetchOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), lsbomSingleTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lsbom", "-sd", bomPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Warnf("failed to create lsbom stdout pipe for %s: %v", bomPath, err)
		return bomFetchOutcome{}
	}
	if err := cmd.Start(); err != nil {
		log.Warnf("failed to start lsbom for %s: %v", bomPath, err)
		return bomFetchOutcome{}
	}

	builder := newBomDigestBuilder()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), lsbomScannerMaxTokenSize)
	for scanner.Scan() {
		builder.addLine(scanner.Text())
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		log.Warnf("lsbom scan failed for %s: %v", bomPath, scanErr)
	}
	if waitErr != nil {
		log.Warnf("lsbom failed for %s: %v", bomPath, waitErr)
	}
	if scanErr != nil || waitErr != nil {
		return bomFetchOutcome{}
	}
	return bomFetchOutcome{Digest: builder.result(), Cacheable: true}
}

// batchLsbom fetches lsbom -sd output for all given BOM paths using direct
// exec.Command invocations only, avoiding shell execution entirely.
func batchLsbom(bomPaths []string) map[string]bomFetchOutcome {
	if len(bomPaths) == 0 {
		return nil
	}

	result := make(map[string]bomFetchOutcome, len(bomPaths))
	for _, bp := range bomPaths {
		result[bp] = singleLsbom(bp)
	}
	return result
}

// appToPkgIndex maps absolute app paths (e.g., "/Applications/zoom.us.app") to the
// package identifier that installed them, derived from BOM data. This replaces the
// expensive per-app `pkgutil --file-info` subprocess calls in applicationsCollector.
type appToPkgIndex struct {
	mu      sync.Mutex
	index   map[string]string // appPath → pkgID
	built   bool
	builtAt time.Time
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
// The index is built on first call and refreshed after defaultBomCacheTTL has elapsed.
func (idx *appToPkgIndex) lookupPkgForApp(appPath string) string {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.built || time.Since(idx.builtAt) >= defaultBomCacheTTL {
		idx.index = buildAppToPkgMap()
		idx.built = true
		idx.builtAt = time.Now()
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
	bomDigests := cache.getBomDigests(bomPaths)

	result := make(map[string]string)
	for _, item := range items {
		digest := bomDigests[item.bomPath]
		prefix := normalizePrefixPath(item.prefixPath)

		for _, appPath := range digest.AppPaths {
			var absPath string
			if prefix == "" {
				absPath = filepath.Join("/", appPath)
			} else {
				absPath = filepath.Join(prefix, appPath)
			}
			// Only index top-level .app bundles directly in /Applications or ~/Applications.
			dir := filepath.Dir(absPath)
			if dir == "/Applications" || strings.HasPrefix(dir, "/Applications/") ||
				strings.HasPrefix(dir, "/Users/") {
				if existingPkgID, exists := result[absPath]; exists && existingPkgID != item.pkgID {
					log.Debugf("app path %s claimed by multiple pkg receipts: existing=%s new=%s", absPath, existingPkgID, item.pkgID)
				}
				result[absPath] = item.pkgID
			}
		}
	}
	return result
}

// Collect reads PKG installer receipts from /var/db/receipts.
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

	// Collect unique BOM paths.
	bomPaths := make([]string, 0, len(receipts))
	seen := make(map[string]bool, len(receipts))
	for _, r := range receipts {
		if !seen[r.bomPath] {
			bomPaths = append(bomPaths, r.bomPath)
			seen[r.bomPath] = true
		}
	}

	// Fetch all BOM data in one batch (cache hit = 0 subprocesses, miss = 1 subprocess per BOM).
	cache := getGlobalBomCache()
	bomDigests := cache.getBomDigests(bomPaths)

	for _, receipt := range receipts {
		digest := bomDigests[receipt.bomPath]
		summary := buildPkgSummaryFromDigest(digest, receipt.prefixPath)
		if entry := buildEntryFromReceipt(receipt, summary, is64Bit); entry != nil {
			entries = append(entries, entry)
		}
	}

	return entries, warnings, nil
}
