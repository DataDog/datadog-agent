// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// applicationsCollector collects software from /Applications directory
type applicationsCollector struct{}

// userAppDir represents a user's Applications directory with associated metadata
type userAppDir struct {
	path     string // Full path to the Applications directory
	username string // Username (empty for system-wide /Applications)
}

// getLocalUsers returns a list of local user home directories by scanning /Users/
// It filters out system directories like Shared, Guest, and hidden directories.
func getLocalUsers() ([]string, []*Warning) {
	var users []string
	var warnings []*Warning

	entries, err := os.ReadDir("/Users")
	if err != nil {
		warnings = append(warnings, warnf("failed to read /Users directory: %v", err))
		return users, warnings
	}

	// Directories to skip - these are not real user home directories
	skipDirs := map[string]bool{
		"Shared":     true,
		"Guest":      true,
		".localized": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip hidden directories (starting with .)
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Skip known system directories
		if skipDirs[name] {
			continue
		}

		userPath := filepath.Join("/Users", name)
		users = append(users, userPath)
	}

	return users, warnings
}

// appPkgLookup holds info needed for parallel pkgutil lookup
type appPkgLookup struct {
	entry   *Entry
	appPath string
}

// populatePkgInfoParallel queries pkgutil for multiple apps in parallel
// Uses a worker pool to limit concurrent pkgutil processes
func populatePkgInfoParallel(items []appPkgLookup) {
	const maxWorkers = 10 // Limit concurrent pkgutil processes

	if len(items) == 0 {
		return
	}

	jobs := make(chan *appPkgLookup, len(items))
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
				if pkgInfo := getPkgInfo(item.appPath); pkgInfo != nil {
					item.entry.InstallSource = installSourcePkg
					item.entry.PkgID = pkgInfo.PkgID
				}
			}
		}()
	}

	wg.Wait()
}

// Collect scans the /Applications directory recursively for installed applications.
// This includes apps in subdirectories like /Applications/SentinelOne/SentinelOne Extensions.app
// It also scans ~/Applications for all local users on the system.
func (c *applicationsCollector) Collect() ([]*Entry, []*Warning, error) {

	var entries []*Entry
	var warnings []*Warning
	var itemsForPublisher []entryWithPath
	var itemsForPkgLookup []appPkgLookup

	// Build list of application directories to scan
	appDirs := []userAppDir{
		{path: "/Applications", username: ""},                  // System-wide applications
		{path: "/System/Applications", username: ""},           // Apple system applications
		{path: "/System/Applications/Utilities", username: ""}, // Apple system utilities
	}

	// Get all local users and add their ~/Applications directories
	localUsers, userWarnings := getLocalUsers()
	warnings = append(warnings, userWarnings...)

	for _, userHome := range localUsers {
		username := filepath.Base(userHome)
		userAppsPath := filepath.Join(userHome, "Applications")

		// Check if the user's Applications directory exists and is accessible
		if info, err := os.Stat(userAppsPath); err == nil && info.IsDir() {
			appDirs = append(appDirs, userAppDir{path: userAppsPath, username: username})
		}
		// If directory doesn't exist or can't be accessed, silently skip
		// (most users won't have ~/Applications)
	}

	for _, appDir := range appDirs {
		// Capture username for this directory (used in closure below)
		currentUsername := appDir.username

		// Use WalkDir for recursive scanning to find .app bundles in subdirectories
		// e.g., /Applications/SentinelOne/SentinelOne Extensions.app
		err := filepath.WalkDir(appDir.path, func(appPath string, d fs.DirEntry, err error) error {
			if err != nil {
				// Skip directories we can't access
				return nil
			}

			// Only process .app bundles (directories ending in .app)
			if !d.IsDir() || !strings.HasSuffix(d.Name(), ".app") {
				return nil
			}

			// Don't descend into .app bundles - they're bundles, not folders to scan
			// We'll process this .app and then skip its contents

			infoPlistPath := filepath.Join(appPath, "Contents", "Info.plist")

			// Check if Info.plist exists (valid app bundle)
			if _, err := os.Stat(infoPlistPath); os.IsNotExist(err) {
				return fs.SkipDir // Skip invalid bundles
			}

			plistData, err := readPlistFile(infoPlistPath)
			if err != nil {
				warnings = append(warnings, warnf("failed to read Info.plist for %s: %v", d.Name(), err))
				return fs.SkipDir
			}

			// Get display name (prefer CFBundleDisplayName, fall back to CFBundleName)
			displayName := plistData["CFBundleDisplayName"]
			if displayName == "" {
				displayName = plistData["CFBundleName"]
			}
			if displayName == "" {
				// Use the app bundle name without .app extension
				displayName = strings.TrimSuffix(d.Name(), ".app")
			}

			// Get version (prefer CFBundleShortVersionString, fall back to CFBundleVersion)
			version := plistData["CFBundleShortVersionString"]
			if version == "" {
				version = plistData["CFBundleVersion"]
			}

			// Get bundle identifier as product code
			bundleID := plistData["CFBundleIdentifier"]

			// Get install date from file modification time
			var installDate string
			if info, err := os.Stat(appPath); err == nil {
				installDate = info.ModTime().UTC().Format(time.RFC3339)
			}

			// Determine the software type and installation source
			// Priority: 1) System app, 2) Mac App Store, 3) PKG installer, 4) Manual (drag-and-drop)
			source := softwareTypeApp
			installSource := installSourceManual
			needsPkgLookup := false

			// Check if this is an Apple system app (from /System/Applications)
			if strings.HasPrefix(appPath, "/System/Applications/") {
				source = softwareTypeSystemApp
				installSource = "" // System apps are pre-installed, no install source
			} else {
				// Check if this is a Mac App Store app by looking for _MASReceipt folder
				// MAS apps store their receipt inside the bundle, not in /var/db/receipts
				masReceiptPath := filepath.Join(appPath, "Contents", "_MASReceipt", "receipt")
				if _, err := os.Stat(masReceiptPath); err == nil {
					source = softwareTypeMAS
					installSource = installSourceMAS
				} else {
					// Not a MAS app or system app - will need to check pkgutil later (in parallel)
					needsPkgLookup = true
				}
			}

			// Determine architecture
			is64Bit := runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"

			// Check bundle integrity
			status := statusInstalled
			brokenReason := checkAppBundleIntegrity(appPath, plistData)
			if brokenReason != "" {
				status = statusBroken
			}

			entry := &Entry{
				DisplayName:   displayName,
				Version:       version,
				InstallDate:   installDate,
				Source:        source,
				ProductCode:   bundleID,
				Status:        status,
				BrokenReason:  brokenReason,
				Is64Bit:       is64Bit,
				InstallSource: installSource,
				InstallPath:   appPath,
				UserSID:       currentUsername, // Set username for per-user apps (empty for system-wide)
			}

			entries = append(entries, entry)
			itemsForPublisher = append(itemsForPublisher, entryWithPath{entry: entry, path: appPath, plistData: plistData})

			// Queue for parallel pkgutil lookup if needed
			if needsPkgLookup {
				itemsForPkgLookup = append(itemsForPkgLookup, appPkgLookup{entry: entry, appPath: appPath})
			}

			// Skip descending into the .app bundle (it's a bundle, not a folder to scan)
			return fs.SkipDir
		})

		if err != nil {
			warnings = append(warnings, warnf("failed to scan directory %s: %v", appDir.path, err))
		}
	}

	// Populate PKG info in parallel for non-MAS apps
	// This queries pkgutil --file-info to determine if the app was installed via PKG
	populatePkgInfoParallel(itemsForPkgLookup)

	// Populate publisher info in parallel using Info.plist extraction
	populatePublishersParallel(itemsForPublisher)

	return entries, warnings, nil
}
