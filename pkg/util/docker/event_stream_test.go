// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubscribe(t *testing.T) {
	state := newEventStreamState()
	assert.Equal(t, 0, len(state.subscribers))

	sub1, err := state.subscribe("listener1", nil)
	assert.NoError(t, err)
	assert.NotNil(t, sub1)
	assert.Equal(t, 1, len(state.subscribers))

	sub2, err := state.subscribe("listener2", nil)
	assert.NoError(t, err)
	assert.NotNil(t, sub2)
	assert.Equal(t, 2, len(state.subscribers))

	_, err = state.subscribe("listener2", nil)
	assert.Equal(t, ErrAlreadySubscribed, err)
	assert.Equal(t, 2, len(state.subscribers))
}

func TestUnsubscribe(t *testing.T) {
	state := newEventStreamState()
	assert.Equal(t, 0, len(state.subscribers))

	_, err := state.subscribe("listener1", nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(state.subscribers))

	err = state.unsubscribe("listener2")
	assert.Equal(t, ErrNotSubscribed, err)
	assert.Equal(t, 1, len(state.subscribers))

	err = state.unsubscribe("listener1")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.subscribers))
}
