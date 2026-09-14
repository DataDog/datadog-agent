// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package auditorimpl

import "os"

func replaceRegistryFile(sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}
