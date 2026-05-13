// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMessageSetTraceContext exercises the formatting in setTraceContext —
// 32-char hex trace ID, decimal span/parent IDs, and the "omit parent_id when
// zero" rule.
func TestMessageSetTraceContext(t *testing.T) {
	var msg message
	msg.setTraceContext(traceContext{
		traceIDLower: 0x1122334455667788,
		traceIDUpper: 0x99aabbccddeeff00,
		spanID:       123,
		parentID:     456,
		valid:        true,
	})

	require.True(t, msg.hasTraceContext())
	require.Equal(t, "99aabbccddeeff001122334455667788", msg.DDTraceID)
	require.Equal(t, "123", msg.DDSpanID)
	require.Equal(t, "456", msg.DDParentID)

	var msg2 message
	msg2.setTraceContext(traceContext{
		traceIDLower: 1,
		spanID:       42,
		parentID:     0, // omitted from output
		valid:        true,
	})
	require.True(t, msg2.hasTraceContext())
	require.Equal(t, "00000000000000000000000000000001", msg2.DDTraceID)
	require.Equal(t, "42", msg2.DDSpanID)
	require.Empty(t, msg2.DDParentID)
}
