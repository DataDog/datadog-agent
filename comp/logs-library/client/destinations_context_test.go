// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
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
