// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"crypto/sha256"
	"fmt"
	"regexp"
)

// ContainerIDPatternStr is the pattern of a container ID
var ContainerIDPatternStr = fmt.Sprintf(`([[:xdigit:]]{%v})`, sha256.Size*2)

// containerIDPattern is the pattern of a container ID
var containerIDPattern = regexp.MustCompile(ContainerIDPatternStr)

// FindContainerID extracts the first sub string that matches the pattern of a container ID
func FindContainerID(s string) string {
	return containerIDPattern.FindString(s)
}
