// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestDeferredTranslationDeletesCapCountsPendingAndReady(t *testing.T) {
	d := newDeferredTranslationDeletes(2)
	assert.True(t, d.add(network.ConnectionTuple{SPort: 1}))
	assert.True(t, d.add(network.ConnectionTuple{SPort: 2}))
	assert.Empty(t, d.rotate(), "nothing was queued before the first rotate")

	// both tuples now wait in ready, so the queue is full even though pending is empty
	assert.False(t, d.add(network.ConnectionTuple{SPort: 3}), "queued past the cap")
	assert.Len(t, d.rotate(), 2)
	assert.True(t, d.add(network.ConnectionTuple{SPort: 3}))
}

func TestDeferredTranslationDeletesNil(t *testing.T) {
	var d *deferredTranslationDeletes
	assert.False(t, d.add(network.ConnectionTuple{}), "a nil queue must make the caller delete now")
	assert.Nil(t, d.rotate())
}
