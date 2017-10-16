// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package forwarder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlock(t *testing.T) {
	e := newBlockedEndpoints()

	before := time.Now()
	e.block("test")
	after := time.Now()
	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, before.Add(blockInterval).Before(e.errorPerEndpoint["test"].until) ||
		before.Add(blockInterval).Equal(e.errorPerEndpoint["test"].until))
	assert.True(t, after.Add(blockInterval).After(e.errorPerEndpoint["test"].until) ||
		after.Add(blockInterval).Equal(e.errorPerEndpoint["test"].until))
}

func TestMaxBlock(t *testing.T) {
	e := newBlockedEndpoints()
	e.block("test")
	e.errorPerEndpoint["test"].nbError = 100

	before := time.Now()
	e.block("test")
	after := time.Now()
	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, before.Add(maxBlockInterval).Before(e.errorPerEndpoint["test"].until) ||
		before.Add(maxBlockInterval).Equal(e.errorPerEndpoint["test"].until))
	assert.True(t, after.Add(maxBlockInterval).After(e.errorPerEndpoint["test"].until) ||
		after.Add(maxBlockInterval).Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblock(t *testing.T) {
	e := newBlockedEndpoints()

	e.block("test")
	require.Contains(t, e.errorPerEndpoint, "test")

	e.unblock("test")
	assert.NotContains(t, e.errorPerEndpoint, "test")
}

func TestUnblockUnknown(t *testing.T) {
	e := newBlockedEndpoints()

	e.unblock("test")
	assert.NotContains(t, e.errorPerEndpoint, "test")
}

func TestIsBlock(t *testing.T) {
	e := newBlockedEndpoints()

	assert.False(t, e.isBlock("test"))

	e.block("test")
	assert.True(t, e.isBlock("test"))

	e.unblock("test")
	assert.False(t, e.isBlock("test"))
}

func TestIsBlockTiming(t *testing.T) {
	e := newBlockedEndpoints()

	// setting an old block
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(-time.Duration(30 * time.Second))}
	assert.False(t, e.isBlock("test"))

	// setting an new block
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(time.Duration(30 * time.Second))}
	assert.True(t, e.isBlock("test"))
}

func TestIsblockUnknown(t *testing.T) {
	e := newBlockedEndpoints()

	assert.False(t, e.isBlock("test"))
}
