// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifacts

import (
	"fmt"

	digest "github.com/opencontainers/go-digest"
)

// DigestPathComponent returns a filesystem-safe directory name for an OCI digest.
func DigestPathComponent(value string) (string, error) {
	parsedDigest, err := digest.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid artifact digest %q: %w", value, err)
	}
	return parsedDigest.Algorithm().String() + "-" + parsedDigest.Encoded(), nil
}
