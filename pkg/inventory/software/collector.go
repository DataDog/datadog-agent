// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package software provides cross-platform software inventory collection.
// This package defines the interfaces and types for collecting software inventory
// information from various sources on the host system, including installed applications,
// their versions, installation dates, and other metadata.
package software

import (
	"errors"
	"fmt"
)

// Collector defines the interface for collecting software entries
// from a specific source or location on the system. Different collectors
// may target different software sources (e.g., Windows Registry, package managers,
// application directories) to provide comprehensive software inventory coverage.
type Collector interface {
	// Collect returns a list of software entries and any warnings encountered
	// during the collection process. This method should be implemented by each
	// collector to gather software information from its specific source.
	// Returns:
	//   - entries: List of software entries found by this collector
	//   - warnings: Non-fatal issues encountered during collection
	//   - error: Fatal error that prevented collection from completing
	Collect() ([]*Entry, []*Warning, error)
}

// Warning represents a non-fatal error during collection
// that should be reported but doesn't prevent the overall collection
// process from completing successfully.
type Warning struct {
	// Message contains a human-readable description of the warning
	Message string
}

func warnf(format string, args ...interface{}) *Warning {
	return &Warning{Message: fmt.Sprintf(format, args...)}
}

// Entry represents a software installation found on the system.
// This structure contains comprehensive metadata about a single software
// application, including identification, versioning, installation details,
// and system-specific information.
type Entry struct {
	// DisplayName is the human-readable name of the software application
	// as it appears to users (e.g., "Microsoft Office 365", "Adobe Photoshop").
	// This field is used for display purposes and software identification.
	DisplayName string `json:"name"`

	// Version is the version string of the software application
	// (e.g., "16.0.1234.56789", "2023.1.2"). This field helps track
	// software versions for security and compliance purposes.
	Version string `json:"version"`

	// InstallDate is the date when the software was installed on the system.
	// The format may vary by platform but is typically in ISO 8601 format
	// or a platform-specific date format (e.g., "2023-01-15T10:30:00Z").
	// This field is optional and may be empty if the installation date
	// cannot be determined.
	InstallDate string `json:"deployment_time,omitempty"`

	// Source indicates the type or source of the software installation
	// (e.g., "desktop", "msstore", "msu"). This field helps categorize
	// software by its installation method or distribution channel.
	Source string `json:"software_type"`

	// UserSID is the Security Identifier of the user who installed the software,
	// particularly relevant for user-specific installations on Windows.
	// This field is optional and may be empty for system-wide installations.
	UserSID string `json:"user,omitempty"`

	// Is64Bit indicates whether the software is a 64-bit application.
	// This field is important for compatibility and system architecture tracking.
	Is64Bit bool `json:"is_64_bit"`

	// Publisher is the name of the software publisher or vendor
	// (e.g., "Microsoft Corporation", "Adobe Inc."). This field helps
	// identify the software vendor for security and compliance purposes.
	Publisher string `json:"publisher"`

	// Status indicates the current deployment status of the software
	// (e.g., "installed", "uninstalling", "failed"). This field tracks
	// the operational state of the software installation.
	Status string `json:"deployment_status"`

	// ProductCode is a unique identifier for the software product,
	// often used in package management systems or installation databases
	// (e.g., Windows Product Code, package identifiers). This field
	// provides a stable identifier for tracking software across systems.
	ProductCode string `json:"product_code"`
}

// GetID returns a unique identifier for the software entry.
// This method provides a consistent way to identify software entries
// across different collection runs and system restarts. The current
// implementation uses the DisplayName as the identifier, but this
// could be enhanced to use more stable identifiers like ProductCode
// when available.
func (se *Entry) GetID() string {
	return se.DisplayName
}

// GetSoftwareInventoryWithCollectors returns a list of software entries using the provided collectors
func GetSoftwareInventoryWithCollectors(collectors []Collector) ([]*Entry, []*Warning, error) {
	var allWarnings []*Warning
	var allEntries []*Entry
	var allErrors error

	// Collect from all sources
	for _, collector := range collectors {
		entries, warnings, err := collector.Collect()

		// Add any warnings from the collector
		allWarnings = append(allWarnings, warnings...)

		if err != nil {
			// Log error but continue with other collectors
			allErrors = errors.Join(allErrors, err)
			continue
		}

		// Add entries to result list
		allEntries = append(allEntries, entries...)
	}

	return allEntries, allWarnings, allErrors
}

// GetSoftwareInventory returns a list of software entries found on the system
func GetSoftwareInventory() ([]*Entry, []*Warning, error) {
	return GetSoftwareInventoryWithCollectors(defaultCollectors())
}
