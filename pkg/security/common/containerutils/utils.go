// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"regexp"
	"strings"
)

// StrictContainerIDPatternStr defines the regexp used to match container IDs
// ([0-9a-fA-F]{64}) is standard container id used pretty much everywhere
// ([0-9a-fA-F]{32}-[0-9]{10}) is container id used by AWS ECS
// ([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}) is container id used by Garden
var StrictContainerIDPatternStr = "^(([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-[0-9]{10})|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}))$"
var strictContainerIDPattern = regexp.MustCompilePOSIX(StrictContainerIDPatternStr)

// WildContainerIDPatternStr is a bit more loose and match within a path string like "/docker/<containerID>"
var WildContainerIDPatternStr = "(([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-[0-9]{10})|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}))"
var wildContainerIDPattern = regexp.MustCompilePOSIX(WildContainerIDPatternStr)

// FindContainerID extracts the first sub string that matches the pattern of a container ID
func FindContainerID(s string) string {
	if strings.Contains(s, "/docker/") || strings.Contains(s, "/kubepods.slice/") {
		return wildContainerIDPattern.FindString(s)
	}
	return strictContainerIDPattern.FindString(s)
}
