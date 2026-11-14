// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPageSize checks the page size reported in agent_metadata against the OS's own source for
// it (see osPageSize). CI runs this package on Linux x64 and arm64, Windows, macOS on Intel and
// Apple silicon, and AIX, which covers 4KiB and 16KiB pages. No CI runner has 64KiB pages, so
// those are covered by the page size suite in test/new-e2e/tests/agent-telemetry.
func TestPageSize(t *testing.T) {
	sndr := makeSenderImpl(t, nil, "agent_telemetry:\n  enabled: true")
	pageSize := sndr.(*senderImpl).metadataPayloadTemplate.PageSize

	assert.Equal(t, osPageSize(t), pageSize)
	assert.GreaterOrEqual(t, pageSize, 4096)
	assert.LessOrEqual(t, pageSize, 65536)
	assert.Zero(t, pageSize&(pageSize-1), "page size %d is not a power of two", pageSize)
}
