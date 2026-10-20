// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package rm_test

import "errors"

// mkfifo is unavailable on Windows; the caller skips before invoking it.
func mkfifo(path string) error {
	return errors.New("mkfifo not supported on windows")
}
