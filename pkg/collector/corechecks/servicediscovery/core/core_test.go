// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPidSet(t *testing.T) {
	set := make(PidSet)

	t.Run("empty set", func(t *testing.T) {
		assert.False(t, set.Has(123))
	})

	t.Run("add and remove", func(t *testing.T) {
		set.Add(123)
		assert.True(t, set.Has(123))

		set.Remove(123)
		assert.False(t, set.Has(123))
	})
}
