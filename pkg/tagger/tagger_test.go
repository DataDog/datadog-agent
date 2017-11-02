// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tagger

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

type DummyCollector struct {
	mock.Mock
}

func (c *DummyCollector) Detect(out chan<- []*collectors.TagInfo) (collectors.CollectionMode, error) {
	args := c.Called(out)
	return args.Get(0).(collectors.CollectionMode), args.Error(1)
}
func (c *DummyCollector) Fetch(entity string) ([]string, []string, error) {
	args := c.Called(entity)
	return args.Get(0).([]string), args.Get(1).([]string), args.Error(2)
}

func (c *DummyCollector) Stream() error {
	args := c.Called()
	return args.Error(0)
}

func (c *DummyCollector) Stop() error {
	args := c.Called()
	return args.Error(0)
}

func (c *DummyCollector) Pull() error {
	args := c.Called()
	return args.Error(0)
}

func NewDummyStreamer() collectors.Collector {
	c := new(DummyCollector)
	c.On("Detect", mock.Anything).Return(collectors.StreamCollection, nil)
	c.On("Stream").Return(nil)
	return c
}

func NewDummyPuller() collectors.Collector {
	c := new(DummyCollector)
	c.On("Detect", mock.Anything).Return(collectors.PullCollection, nil)
	c.On("Pull").Return(nil)
	return c
}

func NewDummyFetcher() collectors.Collector {
	c := new(DummyCollector)
	c.On("Detect", mock.Anything).Return(collectors.FetchOnlyCollection, nil)
	return c
}

func TestInit(t *testing.T) {
	catalog := collectors.Catalog{
		"stream":  NewDummyStreamer,
		"pull":    NewDummyPuller,
		"fetcher": NewDummyFetcher,
	}
	assert.Equal(t, 3, len(catalog))

	tagger, err := newTagger()
	assert.Nil(t, err)
	err = tagger.Init(catalog)
	assert.Nil(t, err)

	assert.Equal(t, 3, len(tagger.fetchers))
	assert.Equal(t, 1, len(tagger.streamers))
	assert.Equal(t, 1, len(tagger.pullers))

	streamer := tagger.streamers["stream"].(*DummyCollector)
	assert.NotNil(t, streamer)
	streamer.AssertCalled(t, "Detect", mock.Anything)

	puller := tagger.pullers["pull"].(*DummyCollector)
	assert.NotNil(t, puller)
	puller.AssertCalled(t, "Detect", mock.Anything)

	fetcher := tagger.fetchers["fetcher"].(*DummyCollector)
	assert.NotNil(t, fetcher)
	fetcher.AssertCalled(t, "Detect", mock.Anything)
}

func TestFetchAllMiss(t *testing.T) {
	catalog := collectors.Catalog{"stream": NewDummyStreamer, "pull": NewDummyPuller}
	tagger, _ := newTagger()
	tagger.Init(catalog)

	streamer := tagger.streamers["stream"].(*DummyCollector)
	assert.NotNil(t, streamer)
	streamer.On("Fetch", "entity_name").Return([]string{"low1"}, []string{}, nil)

	puller := tagger.pullers["pull"].(*DummyCollector)
	assert.NotNil(t, puller)
	puller.On("Fetch", "entity_name").Return([]string{"low2"}, []string{}, nil)

	tags, err := tagger.Tag("entity_name", false)
	assert.Nil(t, err)
	sort.Strings(tags)
	assert.Equal(t, []string{"low1", "low2"}, tags)

	streamer.AssertCalled(t, "Fetch", "entity_name")
	puller.AssertCalled(t, "Fetch", "entity_name")
}

func TestFetchAllCached(t *testing.T) {
	catalog := collectors.Catalog{"stream": NewDummyStreamer, "pull": NewDummyPuller}
	tagger, _ := newTagger()
	tagger.Init(catalog)

	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:       "entity_name",
		Source:       "stream",
		LowCardTags:  []string{"low1"},
		HighCardTags: []string{"high"},
	})
	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:      "entity_name",
		Source:      "pull",
		LowCardTags: []string{"low2"},
	})

	streamer := tagger.streamers["stream"].(*DummyCollector)
	assert.NotNil(t, streamer)
	streamer.On("Fetch", "entity_name").Return([]string{"low1"}, []string{}, nil)

	puller := tagger.pullers["pull"].(*DummyCollector)
	assert.NotNil(t, puller)
	puller.On("Fetch", "entity_name").Return([]string{"low2"}, []string{}, nil)

	tags, err := tagger.Tag("entity_name", true)
	assert.Nil(t, err)
	sort.Strings(tags)
	assert.Equal(t, []string{"high", "low1", "low2"}, tags)

	streamer.AssertNotCalled(t, "Fetch", "entity_name")
	puller.AssertNotCalled(t, "Fetch", "entity_name")
}

func TestFetchOneCached(t *testing.T) {
	catalog := collectors.Catalog{
		"stream":  NewDummyStreamer,
		"pull":    NewDummyPuller,
		"fetcher": NewDummyFetcher,
	}
	tagger, _ := newTagger()
	tagger.Init(catalog)

	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:      "entity_name",
		Source:      "stream",
		LowCardTags: []string{"low1"},
	})

	streamer := tagger.streamers["stream"].(*DummyCollector)
	assert.NotNil(t, streamer)
	streamer.On("Fetch", "entity_name").Return([]string{"low1"}, []string{}, nil)

	puller := tagger.pullers["pull"].(*DummyCollector)
	assert.NotNil(t, puller)
	puller.On("Fetch", "entity_name").Return([]string{"low2"}, []string{}, nil)

	fetcher := tagger.fetchers["fetcher"].(*DummyCollector)
	assert.NotNil(t, fetcher)
	fetcher.On("Fetch", "entity_name").Return([]string{"low3"}, []string{}, nil)

	tags, err := tagger.Tag("entity_name", true)
	assert.Nil(t, err)
	sort.Strings(tags)
	assert.Equal(t, []string{"low1", "low2", "low3"}, tags)

	streamer.AssertNotCalled(t, "Fetch", "entity_name")
	puller.AssertCalled(t, "Fetch", "entity_name")
	fetcher.AssertCalled(t, "Fetch", "entity_name")
}

func TestEmptyEntity(t *testing.T) {
	catalog := collectors.Catalog{
		"fetcher": NewDummyFetcher,
	}
	tagger, _ := newTagger()
	tagger.Init(catalog)

	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:      "entity_name",
		Source:      "stream",
		LowCardTags: []string{"low1"},
	})

	tags, err := tagger.Tag("", true)
	assert.Nil(t, tags)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "empty entity ID")
}
