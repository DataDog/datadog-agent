// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDestinationsContext(t *testing.T) {
	destinationsCtx := NewDestinationsContext()
	assert.Nil(t, destinationsCtx.Context())

	destinationsCtx.Start()
	context := destinationsCtx.Context()
	assert.NotNil(t, context)

	destinationsCtx.Stop()
	assert.NotNil(t, destinationsCtx.Context())

	// We simply make sure that the DestinationsContext correctly cancels its context.
	<-context.Done()

	destinationsCtx.Start()
	assert.NotNil(t, destinationsCtx.Context())
	assert.NotEqual(t, context, destinationsCtx.Context())
}

func TestDestinationsContextChildCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	child := NewDestinationsContext()
	child.StartWithContext(parent)
	child.Stop()
	if child.Context().Err() != context.Canceled {
		t.Fatal("child did not stop")
	}
	if parent.Err() != nil {
		t.Fatal("child stopped the parent")
	}
	child.StartWithContext(parent)
	cancel()
	if child.Context().Err() != context.Canceled {
		t.Fatal("parent cancellation did not stop child")
	}
}
