// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChild_StartsNotAlive(t *testing.T) {
	assert.False(t, NewChild().IsAlive())
}

func TestChild_MarkAliveSetsAlive(t *testing.T) {
	h := NewChild()
	h.MarkAlive()
	assert.True(t, h.IsAlive())
}

func TestChild_MarkDeadClearsAlive(t *testing.T) {
	h := NewChild()
	h.MarkAlive()
	h.MarkDead()
	assert.False(t, h.IsAlive())
}

func TestNoopChildHandle_AlwaysNotAlive(t *testing.T) {
	assert.False(t, NewNoopChildHandle().IsAlive())
}

func TestChild_ZeroValueIsSafe(t *testing.T) {
	var c Child
	assert.False(t, c.IsAlive())
	c.MarkAlive()
	assert.True(t, c.IsAlive())
	c.MarkDead()
	assert.False(t, c.IsAlive())
}
