// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tmplvar provides functions to interact with template variables
package tmplvar

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
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

var tmplVarEnvRegex = regexp.MustCompile(`%%env_.+?%%`)

// ParseTemplateEnvString replaces all %%env_VARIABLE%% from environment
// variables. When there is an error, use the original string
func ParseTemplateEnvString(input string) string {
	input = strings.TrimSpace(input)
	matches := tmplVarEnvRegex.FindAllStringSubmatchIndex(input, -1)
	if len(matches) < 1 {
		return input
	}
	toBeMerged := []string{}
	startIndex := 0
	for _, match := range matches {
		// string before current match
		if startIndex < len(input) {
			toBeMerged = append(toBeMerged, input[startIndex:match[0]])
		}
		//remove placeholder
		stripped := bytes.Map(func(r rune) rune {
			if unicode.IsSpace(r) || r == '%' {
				return -1
			}
			return r
		}, []byte(input[match[0]:match[1]]))
		// get env var
		split := bytes.SplitN(stripped, []byte("_"), 2)
		if len(split) == 2 {
			value, err := getEnvvar(string(split[1]))
			if err == nil {
				toBeMerged = append(toBeMerged, value)
			} else {
				toBeMerged = append(toBeMerged, input[match[0]:match[1]])
			}
		}
		startIndex = match[1]
	}
	//if last part is not template var
	if startIndex < len(input) {
		toBeMerged = append(toBeMerged, input[startIndex:])
	}
	if len(toBeMerged) > 0 {
		return strings.Join(toBeMerged, "")
	}
	return input
}

// getEnvvar returns a system environment variable if found
func getEnvvar(envVar string) (string, error) {
	if len(envVar) == 0 {
		return "", fmt.Errorf("envvar name is missing")
	}
	value, found := os.LookupEnv(envVar)
	if !found {
		return "", fmt.Errorf("failed to retrieve envvar %s", envVar)
	}
	return value, nil
}
