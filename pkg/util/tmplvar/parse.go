// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tmplvar provides functions to interact with template variables
package tmplvar

import (
	"bytes"
	"regexp"
	"unicode"
)

var tmplVarRegex = regexp.MustCompile(`%%.+?%%`)

// TemplateVar is the info for a parsed template variable.
type TemplateVar struct {
	Raw, Name, Key []byte
}

// ParseString returns parsed template variables found in the input string.
func ParseString(s string) []TemplateVar {
	return Parse([]byte(s))
}

// Parse returns parsed template variables found in the input data.
func Parse(b []byte) []TemplateVar {
	var parsed []TemplateVar
	vars := tmplVarRegex.FindAll(b, -1)
	for _, v := range vars {
		name, key := parseTemplateVar(v)
		parsed = append(parsed, TemplateVar{v, name, key})
	}
	return parsed
}

// parseTemplateVar extracts the name of the var and the key (or index if it can be
// cast to an int)
func parseTemplateVar(v []byte) (name, key []byte) {
	stripped := bytes.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '%' {
			return -1
		}
		return r
	}, v)
	split := bytes.SplitN(stripped, []byte("_"), 2)
	name = split[0]
	if len(split) == 2 {
		key = split[1]
	} else {
		key = []byte("")
	}
	return name, key
}
