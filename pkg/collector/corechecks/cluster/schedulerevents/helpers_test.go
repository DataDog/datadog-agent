// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"testing"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

var started = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

// afterSeconds returns the instant that is the given number of seconds after started.
func afterSeconds(seconds int) time.Time {
	return started.Add(time.Duration(seconds) * time.Second)
}

// failingTagger is a tagger whose lookups fail. Only Tag is used by the events.
type failingTagger struct {
	tagger.Component
	err error
}

func (f failingTagger) Tag(types.EntityID, types.TagCardinality) ([]string, error) { return nil, f.err }

// newTestTagger returns the mock tagger, with the tags set by setup.
func newTestTagger(t testing.TB, setup func(taggermock.Mock)) taggermock.Mock {
	t.Helper()
	tagger := taggerfxmock.SetupFakeTagger(t)
	if setup != nil {
		setup(tagger)
	}
	return tagger
}

func newTestIndex() *eventIndex {
	index := newEventIndex()
	index.now = func() time.Time { return started }
	return index
}

// collected gathers the events a handler submits.
type collected struct{ events []event.Event }

func (c *collected) submit(e event.Event) { c.events = append(c.events, e) }

func (c *collected) types() []string {
	var out []string
	for _, e := range c.events {
		out = append(out, e.EventType)
	}
	return out
}

// firstIndex is the index of the first event of a workload in tests.
var firstIndex = int(started.Unix())

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func requireTag(t *testing.T, e event.Event, tag string) {
	t.Helper()
	if !hasTag(e.Tags, tag) {
		t.Errorf("event %s has no tag %q, tags: %v", e.EventType, tag, e.Tags)
	}
}

func rejectTagPrefix(t *testing.T, e event.Event, prefix string) {
	t.Helper()
	for _, tag := range e.Tags {
		if len(tag) >= len(prefix) && tag[:len(prefix)] == prefix {
			t.Errorf("event %s has unexpected tag %q", e.EventType, tag)
		}
	}
}
