// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python || windows

package python

import (
	"io"
)

// Provider provides the functionality to populate the status output
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "py"
}

// Section return the section
func (Provider) Section() string {
	return "py"
}

func (p Provider) getStatusInfo(verbose bool) map[string]interface{} {
	stats := make(map[string]interface{})

	stats["verbose"] = verbose

	return stats
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	return nil
}

// Text renders the text output
func (p Provider) Text(verbose bool, buffer io.Writer) error {
	return nil
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return nil
}
