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
)

type bookmarkSaver struct {
	// inputs
	bookmark          evtbookmark.Bookmark
	bookmarkFrequency int
	saveBookmark      func(bookmarkXML string) error

	// track how often to save
	eventsSinceLastBookmark int
	lastBookmark            string
	mu                      sync.Mutex
}

func (b *bookmarkSaver) updateBookmark(event *evtapi.EventRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Updating and rendering the bookmark is fast, and it makes the "update bookmark at end of check"
	// logic easier by avoiding having to track/save/close the event handle, so just do it every time.
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
		err = b.saveBookmark(b.lastBookmark)
		if err != nil {
			return err
		}
		b.resetBookmarkTracking()
	}
	return nil
}

// saveLastBookmark saves/persists the last unsaved bookmark.
// Used to always save the bookmark before the check ends, regardless of bookmarkFrequency.
func (b *bookmarkSaver) saveLastBookmark() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.bookmarkFrequency > 0 && b.eventsSinceLastBookmark > 0 && b.lastBookmark != "" {
		err := b.saveBookmark(b.lastBookmark)
		b.resetBookmarkTracking()
		return err
	}
	return nil
}

func (b *bookmarkSaver) resetBookmarkTracking() {
	b.lastBookmark = ""
	b.eventsSinceLastBookmark = 0
}
