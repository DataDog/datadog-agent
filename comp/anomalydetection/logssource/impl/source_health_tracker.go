// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"context"
	"sort"
	"sync"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// sourceHealthTracker converts LogSources lifecycle and tailer status into
// periodic Observer health evidence. Multiple LogSource objects can briefly
// represent one identifier while an AD source replaces a generic source; the
// identifier is healthy when any current representative is successful.
type sourceHealthTracker struct {
	logSources *sources.LogSources
	sink       observer.LogSourceHealthObserver
	interval   time.Duration
	sources    map[string]map[*sources.LogSource]struct{}
	stopped    sync.WaitGroup
}

func newSourceHealthTracker(logSources *sources.LogSources, sink observer.LogSourceHealthObserver, interval time.Duration) *sourceHealthTracker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &sourceHealthTracker{
		logSources: logSources,
		sink:       sink,
		interval:   interval,
		sources:    make(map[string]map[*sources.LogSource]struct{}),
	}
}

func (t *sourceHealthTracker) start(ctx context.Context) {
	addedDone := make(chan struct{})
	removedDone := make(chan struct{})
	added, removed := t.logSources.SubscribeAll(addedDone, removedDone)
	t.stopped.Add(1)
	go func() {
		defer t.stopped.Done()
		defer close(addedDone)
		defer close(removedDone)
		ticker := time.NewTicker(t.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case source := <-added:
				if source != nil {
					t.add(source)
					t.sampleIdentifier(logSourceIdentifier(source), time.Now().Unix())
				}
			case source := <-removed:
				if source != nil {
					id := logSourceIdentifier(source)
					t.remove(source)
					t.sampleIdentifier(id, time.Now().Unix())
				}
			case now := <-ticker.C:
				t.sampleAt(now.Unix())
			}
		}
	}()
}

func (t *sourceHealthTracker) wait() { t.stopped.Wait() }

func (t *sourceHealthTracker) add(source *sources.LogSource) {
	id := logSourceIdentifier(source)
	if id == "" {
		return
	}
	if t.sources[id] == nil {
		t.sources[id] = make(map[*sources.LogSource]struct{})
	}
	t.sources[id][source] = struct{}{}
}

func (t *sourceHealthTracker) remove(source *sources.LogSource) {
	id := logSourceIdentifier(source)
	members := t.sources[id]
	delete(members, source)
	if len(members) == 0 {
		delete(t.sources, id)
	}
}

func (t *sourceHealthTracker) sampleAt(timestamp int64) {
	ids := make([]string, 0, len(t.sources))
	for id := range t.sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t.sampleIdentifier(id, timestamp)
	}
}

func (t *sourceHealthTracker) sampleIdentifier(id string, timestamp int64) {
	if id == "" {
		return
	}
	healthy := false
	for source := range t.sources[id] {
		if source.Status != nil && source.Status.IsSuccess() {
			healthy = true
			break
		}
	}
	t.sink.ObserveLogSourceHealth(observer.LogSourceHealthObservation{
		SourceID:  id,
		Timestamp: timestamp,
		Healthy:   healthy,
	})
}

func logSourceIdentifier(source *sources.LogSource) string {
	if source == nil {
		return ""
	}
	if source.Config != nil && source.Config.Identifier != "" {
		return source.Config.Identifier
	}
	if source.Name != "" {
		return source.Name
	}
	if source.Config != nil {
		return source.Config.Source
	}
	return ""
}
