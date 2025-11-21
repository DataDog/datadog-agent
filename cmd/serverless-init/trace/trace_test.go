// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInitSpan(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{
		"env":     "test",
		"service": "my-service",
		"version": "1.0",
	}

	span := InitSpan("test-service", "test.operation", "test-resource", "web", startTime, tags)

	assert.NotNil(t, span)
	assert.Equal(t, "test-service", span.Service)
	assert.Equal(t, "test.operation", span.Name)
	assert.Equal(t, "test-resource", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, startTime, span.Start)
	assert.NotZero(t, span.TraceID)
	assert.NotZero(t, span.SpanID)
	assert.Equal(t, uint64(0), span.ParentID)
	assert.Equal(t, tags, span.Meta)
}

func TestInitSpanGeneratesUniqueIDs(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{}

	span1 := InitSpan("service", "name", "resource", "type", startTime, tags)
	span2 := InitSpan("service", "name", "resource", "type", startTime, tags)

	// TraceIDs and SpanIDs should be different
	assert.NotEqual(t, span1.TraceID, span2.TraceID)
	assert.NotEqual(t, span1.SpanID, span2.SpanID)
}
