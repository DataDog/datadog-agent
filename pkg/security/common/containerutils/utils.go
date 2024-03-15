// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"regexp"
)

// ContainerIDPatternStr defines the regexp used to match container IDs
// ([0-9a-fA-F]{64}) is standard container id used pretty much everywhere
// ([0-9a-fA-F]{32}-[0-9]{10}) is container id used by AWS ECS
// ([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}) is container id used by Garden
var ContainerIDPatternStr = "([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-[0-9]{10})|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4})"

// containerIDPattern is the pattern of a container ID
var containerIDPattern = regexp.MustCompile(ContainerIDPatternStr)

// FindContainerID extracts the first sub string that matches the pattern of a container ID
func FindContainerID(s string) string {
	return containerIDPattern.FindString(s)
}
