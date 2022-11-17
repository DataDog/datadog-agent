// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetricsProvider(t *testing.T) {
	c := newProvider()
	assert.Equal(t, nil, c.getCollector("foo"))

	c.retryCollectors(0)
	assert.Equal(t, nil, c.getCollector("foo"))

	c.RegisterCollector(CollectorMetadata{
		ID:       "dummy1",
		Priority: 10,
		Runtimes: []string{"foo", "bar", "baz"},
		Factory: func() (Collector, error) {
			return dummyCollector{
				id: "dummy1",
			}, nil
		},
	})
	c.RegisterCollector(CollectorMetadata{
		ID:       "dummy2",
		Priority: 9,
		Runtimes: []string{"foo"},
		Factory: func() (Collector, error) {
			return nil, ErrPermaFail
		},
	})

	var dummy3Retries int
	c.RegisterCollector(CollectorMetadata{
		ID:       "dummy3",
		Priority: 9,
		Runtimes: []string{"baz"},
		Factory: func() (Collector, error) {
			if dummy3Retries < 2 {
				dummy3Retries++
				return nil, fmt.Errorf("not yet okay")
			}

			return dummyCollector{
				id: "dummy3",
			}, nil
		},
	})

	// No retry, should still fail
	assert.Equal(t, nil, c.getCollector("foo"))

	// dummy1 should answer to everything
	c.retryCollectors(0)
	fooCollector := c.getCollector("foo")
	barCollector := c.getCollector("bar")
	bazCollector := c.getCollector("baz")
	assert.Equal(t, "dummy1", fooCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", barCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", bazCollector.(dummyCollector).id)

	// dummy3 still not there, dummy2 never ok
	c.retryCollectors(0)
	fooCollector = c.getCollector("foo")
	barCollector = c.getCollector("bar")
	bazCollector = c.getCollector("baz")
	assert.Equal(t, "dummy1", fooCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", barCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", bazCollector.(dummyCollector).id)

	// dummy3 still not there as retry did not really happen due to cache validity
	c.retryCollectors(42 * time.Second)
	fooCollector = c.getCollector("foo")
	barCollector = c.getCollector("bar")
	bazCollector = c.getCollector("baz")
	assert.Equal(t, "dummy1", fooCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", barCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", bazCollector.(dummyCollector).id)

	c.retryCollectors(0)
	fooCollector = c.getCollector("foo")
	barCollector = c.getCollector("bar")
	bazCollector = c.getCollector("baz")
	assert.Equal(t, "dummy1", fooCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", barCollector.(dummyCollector).id)
	assert.Equal(t, "dummy3", bazCollector.(dummyCollector).id)

	// Registering a new collector
	c.RegisterCollector(CollectorMetadata{
		ID:       "dummy4",
		Priority: 8,
		Runtimes: []string{"foo"},
		Factory: func() (Collector, error) {
			return dummyCollector{
				id: "dummy4",
			}, nil
		},
	})

	c.retryCollectors(0)
	fooCollector = c.getCollector("foo")
	barCollector = c.getCollector("bar")
	bazCollector = c.getCollector("baz")
	assert.Equal(t, "dummy4", fooCollector.(dummyCollector).id)
	assert.Equal(t, "dummy1", barCollector.(dummyCollector).id)
	assert.Equal(t, "dummy3", bazCollector.(dummyCollector).id)
}
