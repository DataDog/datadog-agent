// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package paths

import (
	"context"
	"os"
)

// RemoveAll removes path.
func RemoveAll(_ context.Context, path string) error {
	return os.RemoveAll(path)
}
