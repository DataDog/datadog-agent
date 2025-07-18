// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package software provides cross-platform software inventory collection.
package software

import "fmt"

// Collector defines the interface for collecting software entries
type Collector interface {
	// Collect returns a list of software entries and any warnings encountered
	Collect() ([]*Entry, []*Warning, error)
}

// Warning represents a non-fatal error during collection
type Warning struct {
	Message string
}

func warnf(format string, args ...interface{}) *Warning {
	return &Warning{Message: fmt.Sprintf(format, args...)}
}

// Entry represents a software installation
type Entry struct {
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
func (se *Entry) GetID() string {
	return se.DisplayName
}
