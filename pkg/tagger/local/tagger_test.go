// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

type DummyCollector struct {
	mock.Mock
}

func (c *DummyCollector) Detect(_ context.Context, out chan<- []*collectors.TagInfo) (collectors.CollectionMode, error) {
	args := c.Called(out)
	return args.Get(0).(collectors.CollectionMode), args.Error(1)
}
func (c *DummyCollector) Fetch(_ context.Context, entity string) ([]string, []string, []string, error) {
	args := c.Called(entity)
	return args.Get(0).([]string), args.Get(1).([]string), args.Get(2).([]string), args.Error(3)
}

func (c *DummyCollector) Stream() error {
	args := c.Called()
	return args.Error(0)
}

func (c *DummyCollector) Stop() error {
	args := c.Called()
	return args.Error(0)
}

func (c *DummyCollector) Pull(context.Context) error {
	args := c.Called()
	return args.Error(0)
}

func NewDummyStreamer() collectors.Collector {
	c := new(DummyCollector)
	c.On("Detect", mock.Anything).Return(collectors.StreamCollection, nil)
	c.On("Stream").Return(nil)
	c.On("Stop").Return(nil)
	return c
}

func NewDummyPuller() collectors.Collector {
	c := new(DummyCollector)
	c.On("Detect", mock.Anything).Return(collectors.PullCollection, nil)
	c.On("Pull").Return(nil)
	c.On("Stop").Return(nil)
	return c
}

func TestInit(t *testing.T) {
	catalog := collectors.Catalog{
		"stream": NewDummyStreamer,
		"pull":   NewDummyPuller,
	}
	assert.Equal(t, 2, len(catalog))

	tagger := NewTagger(catalog)
	tagger.Init()
	defer tagger.Stop()

	assert.Equal(t, 1, len(tagger.streamers))
	assert.Equal(t, 1, len(tagger.pullers))

	streamer := tagger.streamers["stream"].(*DummyCollector)
	assert.NotNil(t, streamer)
	streamer.AssertCalled(t, "Detect", mock.Anything)

	puller := tagger.pullers["pull"].(*DummyCollector)
	assert.NotNil(t, puller)
	puller.AssertCalled(t, "Detect", mock.Anything)
}

func TestTagBuilder(t *testing.T) {
	catalog := collectors.Catalog{"stream": NewDummyStreamer, "pull": NewDummyPuller}
	tagger := NewTagger(catalog)
	tagger.Init()
	defer tagger.Stop()

	tagger.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:       "entity_name",
			Source:       "stream",
			LowCardTags:  []string{"low1"},
			HighCardTags: []string{"high"},
		},
		{
			Entity:      "entity_name",
			Source:      "pull",
			LowCardTags: []string{"low2"},
		},
	})

	tb := tagset.NewHashlessTagsAccumulator()
	err := tagger.AccumulateTagsFor("entity_name", collectors.HighCardinality, tb)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"high", "low1", "low2"}, tb.Get())
}

func TestFetchAllCached(t *testing.T) {
	catalog := collectors.Catalog{"stream": NewDummyStreamer, "pull": NewDummyPuller}
	tagger := NewTagger(catalog)
	tagger.Init()
	defer tagger.Stop()

	tagger.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:       "entity_name",
			Source:       "stream",
			LowCardTags:  []string{"low1"},
			HighCardTags: []string{"high"},
		},
		{
			Entity:      "entity_name",
			Source:      "pull",
			LowCardTags: []string{"low2"},
		},
	})

	streamer := tagger.streamers["stream"].(*DummyCollector)
	assert.NotNil(t, streamer)
	streamer.On("Fetch", "entity_name").Return([]string{"low1"}, []string{}, nil)

	puller := tagger.pullers["pull"].(*DummyCollector)
	assert.NotNil(t, puller)
	puller.On("Fetch", "entity_name").Return([]string{"low2"}, []string{}, nil)

	tags, err := tagger.Tag("entity_name", collectors.HighCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"high", "low1", "low2"}, tags)

	streamer.AssertNotCalled(t, "Fetch", "entity_name")
	puller.AssertNotCalled(t, "Fetch", "entity_name")

	tags2, err := tagger.Tag("entity_name", collectors.LowCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2"}, tags2)

	streamer.AssertNotCalled(t, "Fetch", "entity_name")
	puller.AssertNotCalled(t, "Fetch", "entity_name")
}

func TestRetryCollector(t *testing.T) {
	ctx := context.Background()

	c := &DummyCollector{}
	retryError := &retry.Error{
		LogicError:    fmt.Errorf("testing"),
		RessourceName: "testing",
		RetryStatus:   retry.FailWillRetry,
	}
	c.On("Detect", mock.Anything).Return(collectors.NoCollection, retryError).Once()

	catalog := collectors.Catalog{
		"fetcher": func() collectors.Collector { return c },
	}
	tagger := NewTagger(catalog)
	tagger.Init()
	defer tagger.Stop()

	assert.Len(t, tagger.candidates, 1)
	c.AssertNumberOfCalls(t, "Detect", 1)

	// Keep trying
	for i := 0; i < 10; i++ {
		c.On("Detect", mock.Anything).Return(collectors.NoCollection, retryError).Once()
		tagger.startCollectors(ctx)
		assert.Len(t, tagger.candidates, 1)
	}
	c.AssertNumberOfCalls(t, "Detect", 11)

	// Okay, you win
	c.On("Detect", mock.Anything).Return(collectors.PullCollection, nil)
	tagger.startCollectors(ctx)
	assert.Len(t, tagger.candidates, 0)
	c.AssertNumberOfCalls(t, "Detect", 12)

	// Don't try again
	tagger.startCollectors(ctx)
	c.AssertNumberOfCalls(t, "Detect", 12)
}

func TestErrNotFound(t *testing.T) {
	c := &DummyCollector{}
	c.On("Detect", mock.Anything).Return(collectors.PullCollection, nil)

	catalog := collectors.Catalog{
		"puller": func() collectors.Collector { return c },
	}
	tagger := NewTagger(catalog)
	tagger.Init()
	defer tagger.Stop()

	// Result should be cached
	c.On("Fetch", mock.Anything).Return([]string{}, []string{}, []string{}, errors.NewNotFound("")).Once()
	_, err := tagger.Tag("invalid", collectors.HighCardinality)
	assert.NoError(t, err)

	// Fetch will not be called again
	c.On("Fetch", mock.Anything).Return([]string{}, []string{}, []string{}, errors.NewNotFound("")).Once()
	_, err = tagger.Tag("invalid", collectors.HighCardinality)
	assert.NoError(t, err)
}

func TestSafeCache(t *testing.T) {
	catalog := collectors.Catalog{"pull": NewDummyPuller}
	tagger := NewTagger(catalog)
	tagger.Init()
	defer tagger.Stop()

	tagger.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:      "entity_name",
			Source:      "pull",
			LowCardTags: []string{"low1", "low2", "low3"},
		},
	})

	// First lookup
	tags, err := tagger.Tag("entity_name", collectors.HighCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2", "low3"}, tags)

	// Let's modify the return value
	tags[0] = "nope"

	// Make sure the cache is not affected
	tags2, err := tagger.Tag("entity_name", collectors.HighCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2", "low3"}, tags2)
}
