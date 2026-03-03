// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ebpftest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracePipeParse(t *testing.T) {
	lines := []string{
		`           <...>-2866    [000] d...2.1   590.130406: bpf_trace_printk: hi\n`,
		`CPU:3 [LOST 497 EVENTS]`,
		`CPU:3 [LOST EVENTS]`,
	}

	for _, line := range lines {
		_, err := parseTraceLine(line)
		assert.NoError(t, err, "parsing trace line %q", line)
	}
}
