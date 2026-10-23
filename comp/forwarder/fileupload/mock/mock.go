// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock file upload forwarder component.
package mock

import (
	"context"
	"fmt"
	"io"
	"sync"

	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Upload is an upload recorded by Mock, with the attachment contents read.
type Upload struct {
	Track       fileupload.TrackType
	Event       []byte
	Attachments []Attachment
}

// Attachment is a recorded attachment.
type Attachment struct {
	Name        string
	Filename    string
	ContentType string
	Content     []byte
}

// Mock records uploads instead of sending them.
type Mock struct {
	mu      sync.Mutex
	uploads []Upload
	err     error
}

// New returns a Mock that accepts every upload.
func New() *Mock {
	return &Mock{}
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(func() fileupload.Component { return New() }),
	)
}

// SetError makes subsequent uploads fail with err, after being recorded.
func (m *Mock) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// Uploads returns the uploads recorded so far.
func (m *Mock) Uploads() []Upload {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Upload(nil), m.uploads...)
}

// Upload records req, reading each attachment once.
func (m *Mock) Upload(_ context.Context, req fileupload.UploadRequest) error {
	upload := Upload{Track: req.Track, Event: append([]byte(nil), req.Event...)}
	for _, a := range req.Attachments {
		content, err := readAttachment(a)
		if err != nil {
			return err
		}
		upload.Attachments = append(upload.Attachments, Attachment{
			Name:        a.Name,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Content:     content,
		})
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploads = append(m.uploads, upload)
	return m.err
}

func readAttachment(a fileupload.Attachment) ([]byte, error) {
	if a.Open == nil {
		return nil, fmt.Errorf("attachment %q has no content", a.Name)
	}
	r, err := a.Open()
	if err != nil {
		return nil, fmt.Errorf("opening attachment %q: %w", a.Name, err)
	}
	defer r.Close()
	return io.ReadAll(r)
}
