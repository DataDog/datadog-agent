// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unwrapTestCapability interface {
	unwrapTestCapability()
}

type unwrapTestCheck struct {
	Check
}

type unwrapCapableCheck struct {
	unwrapTestCheck
}

func (*unwrapCapableCheck) unwrapTestCapability() {}

type unwrapTestWrapper struct {
	Check
}

func (w *unwrapTestWrapper) Unwrap() Check {
	return w.Check
}

type selfUnwrappingCheck struct {
	Check
}

func (c *selfUnwrappingCheck) Unwrap() Check {
	return c
}

func TestAsFindsCapabilityThroughUnwrapChain(t *testing.T) {
	inner := &unwrapCapableCheck{}
	wrapped := &unwrapTestWrapper{Check: &unwrapTestWrapper{Check: inner}}

	got, ok := As[unwrapTestCapability](wrapped)

	require.True(t, ok)
	assert.Same(t, inner, got)
}

func TestAsReturnsFalseWhenCapabilityIsMissing(t *testing.T) {
	wrapped := &unwrapTestWrapper{Check: &unwrapTestCheck{}}

	_, ok := As[unwrapTestCapability](wrapped)

	assert.False(t, ok)
}

func TestAsReturnsFalseForNilCheck(t *testing.T) {
	var c Check

	_, ok := As[unwrapTestCapability](c)

	assert.False(t, ok)
}

func TestAsStopsOnSelfUnwrap(t *testing.T) {
	_, ok := As[unwrapTestCapability](&selfUnwrappingCheck{})

	assert.False(t, ok)
}
