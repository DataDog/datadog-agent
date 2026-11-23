// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsissue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepsAreNumberedFromOne(t *testing.T) {
	steps := Steps("first", "second", "third")
	require.Len(t, steps, 3)
	for i, s := range steps {
		assert.Equal(t, int32(i+1), s.Order)
	}
	assert.Equal(t, "second", steps[1].Text)
	assert.Empty(t, Steps())
}
