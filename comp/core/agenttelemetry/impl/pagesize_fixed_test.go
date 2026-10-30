// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows || aix

package agenttelemetryimpl

import "testing"

// osPageSize returns 4KiB, the base page size of Windows and AIX on every architecture the Agent
// supports. Both also use larger pages (Windows large pages, AIX 64KiB, 16MiB and 16GiB pages per
// memory segment), but page_size reports the base page size, as GetSystemInfo and
// sysconf(_SC_PAGE_SIZE) do.
func osPageSize(_ *testing.T) int {
	return 4096
}
