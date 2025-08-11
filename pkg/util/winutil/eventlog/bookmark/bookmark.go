// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

// Package evtbookmark provides helpers for working with Windows Event Log Bookmarks
package evtbookmark

import (
	"fmt"
	"os"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"golang.org/x/sys/windows"
)

// Bookmark is an interface for handling Windows Event Log Bookmarks
// https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events
type Bookmark interface {
	Handle() evtapi.EventBookmarkHandle
	Update(evtapi.EventRecordHandle) error
	Render() (string, error)
	Close()
}

type bookmark struct {
	// Windows API
	eventLogAPI    evtapi.API
	bookmarkHandle evtapi.EventBookmarkHandle
}

// Option type for option pattern for New bookmark constructor
type Option func(*bookmark) error

// New constructs a new Bookmark.
// Call Close() when done to release resources.
func New(options ...Option) (Bookmark, error) {
	var b bookmark

	for _, o := range options {
		err := o(&b)
		if err != nil {
			return nil, err
		}
	}

	if b.bookmarkHandle == evtapi.EventBookmarkHandle(0) {
		if b.eventLogAPI == nil {
			return nil, fmt.Errorf("event log API not set")
		}
		// Create a new empty bookmark
		bookmarkHandle, err := b.eventLogAPI.EvtCreateBookmark("")
		if err != nil {
			return nil, err
		}
		b.bookmarkHandle = bookmarkHandle
	}

	return &b, nil
}

// WithWindowsEventLogAPI sets the API implementation used by the bookmark
func WithWindowsEventLogAPI(api evtapi.API) Option {
	return func(b *bookmark) error {
		b.eventLogAPI = api
		return nil
	}
}

// FromFile loads a rendered bookmark from a file path
func FromFile(bookmarkPath string) Option {
	return func(b *bookmark) error {
		if b.eventLogAPI == nil {
			return fmt.Errorf("event log API not set")
		}
		if b.bookmarkHandle != evtapi.EventBookmarkHandle(0) {
			return fmt.Errorf("bookmark handle already initialized")
		}
		// Read bookmark from file
		bookmarkXML, err := os.ReadFile(bookmarkPath)
		if err != nil {
			return err
		}
		return FromXML(string(bookmarkXML))(b)
	}
}

// FromXML loads a rendered bookmark
func FromXML(bookmarkXML string) Option {
	return func(b *bookmark) error {
		if b.eventLogAPI == nil {
			return fmt.Errorf("event log API not set")
		}
		if b.bookmarkHandle != evtapi.EventBookmarkHandle(0) {
			return fmt.Errorf("bookmark handle already initialized")
		}
		// Load bookmark XML
		bookmarkHandle, err := b.eventLogAPI.EvtCreateBookmark(bookmarkXML)
		if err != nil {
			return err
		}
		b.bookmarkHandle = bookmarkHandle
		return nil
	}
}

// Handle returns the Windows API handle of the bookmark
func (b *bookmark) Handle() evtapi.EventBookmarkHandle {
	return b.bookmarkHandle
}

// Update the bookmark to the position of the event record for eventHandle
func (b *bookmark) Update(eventHandle evtapi.EventRecordHandle) error {
	if b.eventLogAPI == nil {
		return fmt.Errorf("event log API not set")
	}
	if b.bookmarkHandle == evtapi.EventBookmarkHandle(0) {
		return fmt.Errorf("bookmark handle is not initialized")
	}
	return b.eventLogAPI.EvtUpdateBookmark(b.bookmarkHandle, eventHandle)
}

// Render the bookmark to an XML string
func (b *bookmark) Render() (string, error) {
	if b.eventLogAPI == nil {
		return "", fmt.Errorf("event log API not set")
	}
	if b.bookmarkHandle == evtapi.EventBookmarkHandle(0) {
		return "", fmt.Errorf("bookmark handle is not initialized")
	}
	// Render bookmark
	buf, err := b.eventLogAPI.EvtRenderBookmark(b.bookmarkHandle)
	if err != nil {
		return "", err
	} else if len(buf) == 0 {
		return "", fmt.Errorf("Bookmark is empty")
	}

	// Convert to string
	return windows.UTF16ToString(buf), nil
}

// Close this bookmark and release resources used.
func (b *bookmark) Close() {
	if b.eventLogAPI == nil {
		return
	}
	if b.bookmarkHandle != evtapi.EventBookmarkHandle(0) {
		evtapi.EvtCloseBookmark(b.eventLogAPI, b.bookmarkHandle)
		b.bookmarkHandle = evtapi.EventBookmarkHandle(0)
	}
}

// FromLatestEvent creates a bookmark pointing to the most recent event matching the channel/query.
// This prevents the amnesia bug where events between startup and first pull are lost when starting
// from "now". Returns an empty bookmark if no events exist. An error is only returned if the
// underlying API calls fail unexpectedly.
//
// The Windows Event Log API (EvtQuery) automatically handles both single-channel queries and
// multi-channel XML QueryList queries, so no special handling is needed.
func FromLatestEvent(api evtapi.API, channelPath, query string) (Bookmark, error) {
	// Query for the most recent event
	// EvtQuery handles single/multi channel query selection automatically
	resultSet, err := api.EvtQuery(0, channelPath, query,
		evtapi.EvtQueryChannelPath|evtapi.EvtQueryReverseDirection)
	if err != nil {
		return nil, fmt.Errorf("EvtQuery failed: %w", err)
	}
	defer evtapi.EvtCloseResultSet(api, resultSet)

	// Get one event (the most recent due to reverse direction)
	handles := make([]evtapi.EventRecordHandle, 1)
	returned, err := api.EvtNext(resultSet, handles, 1, 1000) // 1 second timeout
	if err != nil {
		if err == windows.ERROR_NO_MORE_ITEMS {
			// No events in the log - return empty bookmark
			return New(WithWindowsEventLogAPI(api))
		}
		return nil, fmt.Errorf("EvtNext failed: %w", err)
	}

	if len(returned) == 0 {
		// No events available - return empty bookmark
		return New(WithWindowsEventLogAPI(api))
	}
	defer evtapi.EvtCloseRecord(api, returned[0])

	// Create and update bookmark with the most recent event
	bookmark, err := New(WithWindowsEventLogAPI(api))
	if err != nil {
		return nil, fmt.Errorf("failed to create bookmark: %w", err)
	}

	if err := bookmark.Update(returned[0]); err != nil {
		bookmark.Close()
		return nil, fmt.Errorf("failed to update bookmark with latest event: %w", err)
	}

	return bookmark, nil
}
