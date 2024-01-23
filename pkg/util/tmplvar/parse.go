// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tmplvar provides functions to interact with template variables
package tmplvar

import (
	"regexp"
)

var tmplVarRegex = regexp.MustCompile(`%%.+?%%`)

// TemplateVar is the info for a parsed template variable.
type TemplateVar struct {
	Raw, Name, Key []byte
}

// ParseString returns parsed template variables found in the input string.
func ParseString(s string) []TemplateVar {
	panic("not called")
}

// Parse returns parsed template variables found in the input data.
func Parse(b []byte) []TemplateVar {
	panic("not called")
}

// parseTemplateVar extracts the name of the var and the key (or index if it can be
// cast to an int)
func parseTemplateVar(v []byte) (name, key []byte) {
	panic("not called")
}
