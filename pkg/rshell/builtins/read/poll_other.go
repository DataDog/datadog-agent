// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !unix

package read

import "github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/winpoll"

// pollInputNonConsuming dispatches to the platform-specific implementation
// for non-Unix builds. On Windows, builtins/internal/winpoll uses
// GetFileType + PeekNamedPipe / GetNumberOfConsoleInputEvents to
// report buffered-data availability without consuming input. On other
// non-Unix platforms (e.g. plan9), the stub returns supported=false and
// the caller falls back to a conservative Code 1.
func pollInputNonConsuming(fd uintptr) (available, supported bool) {
	return winpoll.PollNonConsuming(fd)
}
