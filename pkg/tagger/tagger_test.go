// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tagger

import (
	// stdlib
	"sort"
	"sync"
	"testing"

	// 3p
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

/////////////////////////////// Dummy collectors for mock testing ///////////////////////////////

type DummyStreamer struct {
	sync.Mutex
	Inited      int
	Started     int
	Stopped     int
	Fetched     int
	FetchedWith string
}

func (c *DummyStreamer) Detect(out chan<- []*collectors.TagInfo) (collectors.CollectionMode, error) {
	c.Inited++
	return collectors.StreamCollection, nil
}
func (c *DummyStreamer) Fetch(entity string) ([]string, []string, error) {
	c.Fetched++
	c.FetchedWith = entity
	return []string{"low"}, []string{"high"}, nil
}

func (c *DummyStreamer) Stream() error {
	c.Lock()
	c.Started++
	c.Unlock()
	return nil
}
func (c *DummyStreamer) Stop() error {
	c.Lock()
	c.Stopped++
	c.Unlock()
	return nil
}

type DummyPuller struct {
	sync.Mutex
	Inited      int
	Pulled      int
	Fetched     int
	FetchedWith string
}

func (c *DummyPuller) Detect(out chan<- []*collectors.TagInfo) (collectors.CollectionMode, error) {
	c.Inited++
	return collectors.PullCollection, nil
}
func (c *DummyPuller) Fetch(entity string) ([]string, []string, error) {
	c.Fetched++
	c.FetchedWith = entity
	return []string{"low2"}, []string{}, nil
}

func (c *DummyPuller) Pull() error {
	c.Lock()
	c.Pulled++
	c.Unlock()
	return nil
}

type Dummies struct {
	Streamer *DummyStreamer
	Puller   *DummyPuller
}

func (d *Dummies) getDummyStreamer() collectors.Collector {
	return d.Streamer
}

func (d *Dummies) getDummyPuller() collectors.Collector {
	return d.Puller
}

/////////////////////////////// Test functions ///////////////////////////////

func TestInit(t *testing.T) {
	d := &Dummies{&DummyStreamer{}, &DummyPuller{}}
	catalog := collectors.Catalog{"stream": d.getDummyStreamer, "pull": d.getDummyPuller}
	require.Equal(t, 2, len(catalog))

	tagger, err := NewTagger()
	require.Equal(t, nil, err)
	err = tagger.Init(catalog)
	require.Equal(t, nil, err)

	require.Equal(t, 2, len(tagger.fetchers))
	require.Equal(t, 1, len(tagger.streamers))
	require.Equal(t, 1, len(tagger.pullers))

	require.Equal(t, 1, d.Streamer.Inited)
	require.Equal(t, 1, d.Streamer.Inited)
	require.Equal(t, 0, d.Streamer.Stopped)
	require.Equal(t, 0, d.Streamer.Fetched)

	require.Equal(t, 1, d.Puller.Inited)
	require.Equal(t, 0, d.Puller.Fetched)
}

func TestFetchAllMiss(t *testing.T) {
	d := &Dummies{&DummyStreamer{}, &DummyPuller{}}
	catalog := collectors.Catalog{"stream": d.getDummyStreamer, "pull": d.getDummyPuller}
	require.Equal(t, 2, len(catalog))
	tagger, _ := NewTagger()
	tagger.Init(catalog)

	tags, err := tagger.Tag("entity_name", false)
	require.Equal(t, 1, d.Streamer.Fetched)
	require.Equal(t, "entity_name", d.Streamer.FetchedWith)
	require.Equal(t, 1, d.Puller.Fetched)
	require.Equal(t, "entity_name", d.Puller.FetchedWith)

	require.Equal(t, nil, err)
	sort.Strings(tags)
	require.Equal(t, []string{"low", "low2"}, tags)
}

func TestFetchAllCached(t *testing.T) {
	d := &Dummies{&DummyStreamer{}, &DummyPuller{}}
	catalog := collectors.Catalog{"stream": d.getDummyStreamer, "pull": d.getDummyPuller}
	require.Equal(t, 2, len(catalog))
	tagger, _ := NewTagger()
	tagger.Init(catalog)

	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:       "entity_name",
		Source:       "stream",
		LowCardTags:  []string{"low"},
		HighCardTags: []string{"high"},
	})
	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:      "entity_name",
		Source:      "pull",
		LowCardTags: []string{"low2"},
	})

	tags, err := tagger.Tag("entity_name", true)
	require.Equal(t, 0, d.Streamer.Fetched)
	require.Equal(t, 0, d.Puller.Fetched)

	require.Equal(t, nil, err)
	sort.Strings(tags)
	require.Equal(t, []string{"high", "low", "low2"}, tags)
}

func TestFetchOneCached(t *testing.T) {
	d := &Dummies{&DummyStreamer{}, &DummyPuller{}}
	catalog := collectors.Catalog{"stream": d.getDummyStreamer, "pull": d.getDummyPuller}
	require.Equal(t, 2, len(catalog))
	tagger, _ := NewTagger()
	tagger.Init(catalog)

	tagger.tagStore.processTagInfo(&collectors.TagInfo{
		Entity:      "entity_name",
		Source:      "stream",
		LowCardTags: []string{"low"},
	})

	tags, err := tagger.Tag("entity_name", false)
	require.Equal(t, 0, d.Streamer.Fetched)
	require.Equal(t, 1, d.Puller.Fetched)

	require.Equal(t, nil, err)
	sort.Strings(tags)
	require.Equal(t, []string{"low", "low2"}, tags)
}
