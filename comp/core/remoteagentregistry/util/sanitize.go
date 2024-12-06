// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util provides utility functions for the remoteagent component.
package util

import (
	"regexp"
	"strings"
)

var agentIDSanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9-_]`)
var fileNameSanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9-_\.]`)

// SanitizeAgentID sanitizes a string to be used as an agent ID.
//
// All characters are not ASCII alphanumerics, underscores, or hyphens are replaced with an underscore, and the string
// is converted to lowercase.
func SanitizeAgentID(agentID string) string {
	agentID = agentIDSanitizeRegex.ReplaceAllString(agentID, "_")
	return strings.ToLower(agentID)
}

// SanitizeFileName sanitizes a string to be used as a file name.
//
// All characters that are not ASCII alphanumerics, underscores, or hyphens are replaced with an underscore, and the
// string is trimmed of extraneous whitespace and limited to 255 characters in length.
func SanitizeFileName(fileName string) string {
	fileName = fileNameSanitizeRegex.ReplaceAllString(fileName, "_")
	fileName = strings.TrimSpace(fileName)
	if len(fileName) > 255 {
		fileName = fileName[:255]
	}

	return fileName
}
