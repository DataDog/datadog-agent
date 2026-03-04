// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package interp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTail_WindowsReservedNames(t *testing.T) {
	reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM9", "LPT1", "LPT9"}
	for _, name := range reservedNames {
		t.Run(name, func(t *testing.T) {
			_, stderr, ec := runTail(t, fmt.Sprintf("tail %s", name))
			assert.Equal(t, 1, ec)
			assert.Contains(t, stderr, "reserved")
		})
	}
}
