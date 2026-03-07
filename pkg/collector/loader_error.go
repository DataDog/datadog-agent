// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import "strings"

// LoaderError holds the per-loader errors encountered while trying to load a
// single check instance. The Config field identifies the check configuration
// that failed to load, and Errors maps each loader name to the error message
// it produced.
type LoaderError struct {
	Config string
	Errors map[string]string // loader name -> error message
}

// Error returns a human-readable summary of all loader errors, preserving the
// format previously produced by the ad-hoc strings.Builder accumulation:
//
//	"loaderA: err1; loaderB: err2; "
func (e *LoaderError) Error() string {
	var b strings.Builder
	for loaderName, errMsg := range e.Errors {
		b.WriteString(loaderName)
		b.WriteString(": ")
		b.WriteString(errMsg)
		b.WriteString("; ")
	}
	return b.String()
}
