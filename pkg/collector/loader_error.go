// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import "strings"

// LoaderError records which loaders were tried and failed for a given check config.
// It implements the error interface so it can be used directly in error-handling paths.
type LoaderError struct {
	// Config is the name of the check configuration that could not be loaded.
	Config string
	// Errors maps each loader name to the error message it produced.
	Errors map[string]string // loader name -> error message
}

// Error returns a human-readable summary of all loader failures, preserving the
// format previously produced by the ad-hoc strings.Builder accumulation:
//
//	"<loader>: <msg>; <loader>: <msg>; "
func (e *LoaderError) Error() string {
	var b strings.Builder
	for loaderName, msg := range e.Errors {
		b.WriteString(loaderName)
		b.WriteString(": ")
		b.WriteString(msg)
		b.WriteString("; ")
	}
	return b.String()
}

// Add records a failure for the given loader.
func (e *LoaderError) Add(loaderName, msg string) {
	if e.Errors == nil {
		e.Errors = make(map[string]string)
	}
	e.Errors[loaderName] = msg
}
