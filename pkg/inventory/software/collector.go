// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package softwareinventory software provides cross-platform software inventory collection. This file contains Windows-specific logic.
package softwareinventory

import "fmt"

// SoftwareCollector defines the interface for collecting software entries
type SoftwareCollector interface {
	// Collect returns a list of software entries and any warnings encountered
	Collect() ([]*SoftwareEntry, []*Warning, error)
}

// Warning represents a non-fatal error during collection
type Warning struct {
	Message string
}

func warnf(format string, args ...interface{}) *Warning {
	return &Warning{Message: fmt.Sprintf(format, args...)}
}

// SoftwareEntry represents a software installation
type SoftwareEntry struct {
	DisplayName string `json:"name"`
	Version     string `json:"version"`
	InstallDate string `json:"deployment_time,omitempty"`
	Source      string `json:"software_type"`
	UserSID     string `json:"user,omitempty"`
	Is64Bit     bool   `json:"is_64_bit"`
	Publisher   string `json:"publisher"`
	Status      string `json:"deployment_status"`
	ProductCode string `json:"product_code"`
}

// GetID returns a unique identifier for the software entry
func (se *SoftwareEntry) GetID() string {
	return se.DisplayName
}
