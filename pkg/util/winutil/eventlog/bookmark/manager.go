// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package evtbookmark

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// Manager handles bookmark persistence with frequency-based saving.
//
// Usage pattern:
//  1. Create manager with NewManager()
//  2. Call UpdateAndSave() as events are processed (respects frequency)
//  3. Call Save() periodically or on shutdown (always saves)
//  4. Call Close() to clean up resources
type Manager interface {
	// UpdateAndSave updates the bookmark with an event and saves according to
	// the configured frequency. Use this for normal event processing.
	UpdateAndSave(eventHandle evtapi.EventRecordHandle) error

	// Save immediately saves the current bookmark, ignoring frequency.
	// Use this for periodic checkpoints and before shutdown.
	Save() error

	// Close cleans up resources including closing the bookmark handle.
	Close()
}

// Saver interface abstracts bookmark persistence mechanisms.
// Different implementations can save to persistent cache, auditor registry, etc.
type Saver interface {
	Save(bookmarkXML string) error
	Load() (string, error)
}

// Config contains configuration for creating a BookmarkManager.
type Config struct {
	API               evtapi.API
	Saver             Saver
	BookmarkFrequency int // 0 = save every event, >0 = save every N events
}

// manager implements the Manager interface.
type manager struct {
	api               evtapi.API
	saver             Saver
	bookmarkFrequency int

	// State
	bookmark            Bookmark
	eventsSinceLastSave int
	mu                  sync.Mutex
}

// NewManager creates a new BookmarkManager with the given configuration.
func NewManager(config Config) Manager {
	return &manager{
		api:               config.API,
		saver:             config.Saver,
		bookmarkFrequency: config.BookmarkFrequency,
	}
}

// UpdateAndSave updates the bookmark with the given event and saves according to frequency.
func (m *manager) UpdateAndSave(eventHandle evtapi.EventRecordHandle) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create bookmark on first event if we don't have one yet
	if m.bookmark == nil {
		bookmark, err := New(WithWindowsEventLogAPI(m.api))
		if err != nil {
			return fmt.Errorf("failed to create bookmark: %w", err)
		}
		m.bookmark = bookmark
		log.Debug("Created bookmark for first event")
	}

	// Update bookmark with the event
	err := m.bookmark.Update(eventHandle)
	if err != nil {
		return fmt.Errorf("failed to update bookmark: %w", err)
	}

	// Track events since last save for frequency-based saving
	m.eventsSinceLastSave++

	// Save according to frequency
	// If bookmarkFrequency is 0, save on every event
	// If bookmarkFrequency > 0, save every N events
	shouldSave := (m.bookmarkFrequency == 0) || (m.eventsSinceLastSave >= m.bookmarkFrequency)
	if shouldSave {
		err = m.saveBookmarkInternal()
		if err != nil {
			return err
		}
	}

	return nil
}

// Save immediately saves the current bookmark, ignoring frequency.
// This should be called periodically (e.g., on each check run) or before shutdown.
func (m *manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.bookmark == nil {
		return nil // Nothing to save
	}

	// Only save if there are unsaved events
	if m.eventsSinceLastSave == 0 {
		return nil // Already saved
	}

	return m.saveBookmarkInternal()
}

// Close implements Manager.Close.
func (m *manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.bookmark != nil {
		m.bookmark.Close()
		m.bookmark = nil
	}
}

// saveBookmarkInternal saves the current bookmark and resets counters.
// Must be called with mutex held.
func (m *manager) saveBookmarkInternal() error {
	// Render the bookmark to XML
	bookmarkXML, err := m.bookmark.Render()
	if err != nil {
		return fmt.Errorf("failed to render bookmark: %w", err)
	}

	// Persist the bookmark
	err = m.saver.Save(bookmarkXML)
	if err != nil {
		return fmt.Errorf("failed to save bookmark: %w", err)
	}

	// Reset counter after successful save
	m.eventsSinceLastSave = 0

	return nil
}
