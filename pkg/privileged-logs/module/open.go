// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
)

// openPathWithoutSymlinks opens a file by traversing each path component with
// O_NOFOLLOW.  It is a thin wrapper around common.OpenPathWithoutSymlinks so
// that the implementation is shared with the client side.
func openPathWithoutSymlinks(path string) (*os.File, error) {
	return common.OpenPathWithoutSymlinks(path)
}
