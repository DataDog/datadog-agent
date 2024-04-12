// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && !windows

package python

import (
	"io"
	// "github.com/DataDog/datadog-agent/comp/core/status"
)

// //go:embed status_templates
// var templatesFS embed.FS

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

// JSON populates the status map
func (p Provider) JSON(verbose bool, stats map[string]interface{}) error {
	// stats := make(map[string]interface{})

	py_stats, err := GetPythonInterpreterMemoryUsage()

	if err != nil {
		return err
	}

	stats[""] = py_stats

	stats["verbose"] = verbose

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
