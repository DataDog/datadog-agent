// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

type bookmarkSaver struct {
	// inputs
	sub               evtsubscribe.PullSubscription
	bookmark          evtbookmark.Bookmark
	bookmarkFrequency int
	save              func(bookmarkXML string) error

	// track how often to save
	eventsSinceLastBookmark int
	lastBookmark            string
	mu                      sync.Mutex

	addBookmarkToSubOnce sync.Once
}

func (b *bookmarkSaver) updateBookmark(event *evtapi.EventRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Updating and rendering the bookmark is fast, and it makes the "update bookmark at end of check"
	// logic easier by avoiding having to conditionally track/save/close the event handle, so just do it every time.
	// DuplicateHandle() does not support event log handles, so we can't use it to separate the event
	// from the subscription.
	err := b.bookmark.Update(event.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to update bookmark: %w", err)
	}

	bookmarkXML, err := b.bookmark.Render()
	if err != nil {
		return fmt.Errorf("failed to render bookmark XML: %w", err)
	}
	b.lastBookmark = bookmarkXML

	// The bookmark is only saved/persisted according to the bookmarkFrequency
	b.eventsSinceLastBookmark++
	if b.bookmarkFrequency > 0 && b.eventsSinceLastBookmark >= b.bookmarkFrequency {
		return b.saveBookmark(b.lastBookmark)
	}
	return nil
}

// saveLastBookmark saves/persists the last unsaved bookmark.
// Used to always save the bookmark before the check ends, regardless of bookmarkFrequency.
func (b *bookmarkSaver) saveLastBookmark() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.bookmarkFrequency > 0 && b.eventsSinceLastBookmark > 0 && b.lastBookmark != "" {
		return b.saveBookmark(b.lastBookmark)
	}
	return nil
}

func (b *bookmarkSaver) saveBookmark(bookmarkXML string) error {
	err := b.save(bookmarkXML)
	if err != nil {
		return err
	}

	// bookmark saved successfully so we can reset our counters
	b.resetBookmarkTracking()

	// If we don't have a bookmark when we create the subscription we have
	// to add it later once we've updated it at least once.
	b.addBookmarkToSubOnce.Do(func() {
		b.sub.SetBookmark(b.bookmark)
	})

	return nil
}

func (b *bookmarkSaver) resetBookmarkTracking() {
	b.lastBookmark = ""
	b.eventsSinceLastBookmark = 0
}
