// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

// Warnings represent the warnings in the config
type Warnings struct {
	Errors []error
}

// NewWarnings creates a new Warnings instance
func NewWarnings(errors []error) *Warnings {
	return &Warnings{
		Errors: errors,
	}
}

// Count returns the number of errors in the Warnings
func (w *Warnings) Count() int {
	return len(w.Errors)
}
