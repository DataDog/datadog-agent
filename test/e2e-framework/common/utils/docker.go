// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"fmt"
	"strings"
)

func BuildDockerImagePath(dockerRepository string, imageVersion string) string {
	return fmt.Sprintf("%s:%s", dockerRepository, imageVersion)
}

func ParseImageReference(imageRef string) (imagePath string, tag string) {
	lastColonIdx := strings.LastIndex(imageRef, ":")
	if lastColonIdx > 0 &&
		lastColonIdx < len(imageRef)-1 &&
		// Check not part of registry address (e.g., "registry:5000/image")
		!strings.Contains(imageRef[lastColonIdx:], "/") {
		imagePath = imageRef[:lastColonIdx]
		tag = imageRef[lastColonIdx+1:]
	} else {
		imagePath = imageRef
		tag = "latest"
	}

	// Remove trailing ":" if image name has one.
	if imagePath[len(imagePath)-1:] == ":" {
		imagePath = imagePath[:len(imagePath)-1]
	}
	return
}
