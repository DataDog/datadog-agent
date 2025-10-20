// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package doctorimpl

import (
	"fmt"
	"io"
)

// StatusProvider implements status.HeaderProvider for the doctor component
type StatusProvider struct{}

// Index returns the order in which this header should be displayed
func (s *StatusProvider) Index() int {
	return 0
}

// Name returns the name to display in the header
func (s *StatusProvider) Name() string {
	return "Doctor"
}

// JSON populates the status map
func (s *StatusProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats["doctor_available"] = true
	stats["doctor_endpoint"] = "/agent/doctor"
	return nil
}

// Text renders the text output
func (s *StatusProvider) Text(_ bool, buffer io.Writer) error {
	fmt.Fprintf(buffer, "  Doctor Status: Available at /agent/doctor\n")
	return nil
}

// HTML renders the HTML output
func (s *StatusProvider) HTML(_ bool, buffer io.Writer) error {
	fmt.Fprintf(buffer, "<div>Doctor Status: Available at <code>/agent/doctor</code></div>\n")
	return nil
}
