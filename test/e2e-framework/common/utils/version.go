// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "strings"

// ParseKubernetesVersion extracts the semantic version from a Kubernetes version string.
// It handles versions with image SHA suffixes (e.g., "v1.32.0@sha256:abc123") by returning
// only the version part (e.g., "v1.32.0").
func ParseKubernetesVersion(version string) string {
	// Split on @ to remove any SHA suffix
	if before, _, ok := strings.Cut(version, "@"); ok {
		return before
	}
	return version
}
