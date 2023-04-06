// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtbookmark

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"golang.org/x/sys/windows"
)

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

func New(options ...func(*bookmark) error) (*bookmark, error) {
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

func WithWindowsEventLogAPI(api evtapi.API) func(*bookmark) error {
	return func(b *bookmark) error {
		b.eventLogAPI = api
		return nil
	}
}

func FromFile(bookmarkPath string) func(*bookmark) error {
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

func FromXML(bookmarkXML string) func(*bookmark) error {
	return func(b *bookmark) error {
		if b.eventLogAPI == nil {
			return fmt.Errorf("event log API not set")
		}
		if b.bookmarkHandle != evtapi.EventBookmarkHandle(0) {
			return fmt.Errorf("bookmark handle already initialized")
		}
		// Load bookmark XML
		bookmarkHandle, err := b.eventLogAPI.EvtCreateBookmark(string(bookmarkXML))
		if err != nil {
			return err
		}
		b.bookmarkHandle = bookmarkHandle
		return nil
	}
}

func (b *bookmark) Handle() evtapi.EventBookmarkHandle {
	return b.bookmarkHandle
}

func (b *bookmark) Update(eventHandle evtapi.EventRecordHandle) error {
	if b.eventLogAPI == nil {
		return fmt.Errorf("event log API not set")
	}
	if b.bookmarkHandle == evtapi.EventBookmarkHandle(0) {
		return fmt.Errorf("bookmark handle is not initialized")
	}
	return b.eventLogAPI.EvtUpdateBookmark(b.bookmarkHandle, eventHandle)
}

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
	} else if buf == nil || len(buf) == 0 {
		return "", fmt.Errorf("Bookmark is empty")
	}

	// Convert to string
	return windows.UTF16ToString(buf), nil
}

func (b *bookmark) Close() {
	if b.eventLogAPI == nil {
		return
	}
	if b.bookmarkHandle != evtapi.EventBookmarkHandle(0) {
		evtapi.EvtCloseBookmark(b.eventLogAPI, b.bookmarkHandle)
		b.bookmarkHandle = evtapi.EventBookmarkHandle(0)
	}
}
