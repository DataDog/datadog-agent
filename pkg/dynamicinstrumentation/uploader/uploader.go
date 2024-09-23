// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package uploader provides functionality for uploading events and diagnostic
// information to the DataDog backend
package uploader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

// LogUploader is the interface for uploading Dynamic Instrumentation logs
type LogUploader interface {
	Enqueue(item *ditypes.SnapshotUpload) bool
}

// DiagnosticUploader is the interface for uploading Dynamic Instrumentation
// diagnostic information
type DiagnosticUploader interface {
	Enqueue(item *ditypes.DiagnosticUpload) bool
}

// Uploader is a generic form of uploader functionality
type Uploader[T any] struct {
	buffer chan *T
	client *http.Client

	batchSize  int
	uploadMode UploadMode
}

// UploadMode reflects the kind of data that is being uploaded
type UploadMode bool

const (
	// UploadModeDiagnostic means the data being uploaded is diagnostic information
	UploadModeDiagnostic UploadMode = true
	//UploadModeLog means the data being uploaded is logs
	UploadModeLog UploadMode = false
)

func startDiagnosticUploader(dm *diagnostics.DiagnosticManager) *Uploader[ditypes.DiagnosticUpload] {
	u := NewUploader[ditypes.DiagnosticUpload](UploadModeDiagnostic)
	go func() {
		for diagnostic := range dm.Updates {
			u.Enqueue(diagnostic)
		}
	}()
	return u
}

// NewLogUploader creates a new log uploader
func NewLogUploader() *Uploader[ditypes.SnapshotUpload] {
	return NewUploader[ditypes.SnapshotUpload](UploadModeLog)
}

// NewDiagnosticUploader creates a new diagnostic uploader
func NewDiagnosticUploader() *Uploader[ditypes.DiagnosticUpload] {
	return startDiagnosticUploader(diagnostics.Diagnostics)
}

// NewUploader creates a new uploader of a specified generic type
func NewUploader[T any](mode UploadMode) *Uploader[T] {
	u := &Uploader[T]{
		buffer: make(chan *T, 100),
		client: &http.Client{},

		batchSize:  100,
		uploadMode: mode,
	}
	go u.processBuffer()
	return u
}

// Enqueue enqueues data to be uploaded. It's return value reflects whether
// or not the upload queue was full
func (u *Uploader[T]) Enqueue(item *T) bool {
	select {
	case u.buffer <- item:
		return true
	default:
		log.Infof("Uploader buffer full, dropping message %+v", item)
		return false
	}
}

func (u *Uploader[T]) processBuffer() {
	flushTimer := time.NewTicker(1 * time.Second)
	defer flushTimer.Stop()

	batch := make([]*T, 0, 5)

	for {
		select {
		case item := <-u.buffer:
			batch = append(batch, item)
			if len(batch) >= u.batchSize {
				batchCopy := make([]*T, len(batch))
				copy(batchCopy, batch)
				go u.uploadBatch(batchCopy)
				batch = batch[:0]
				flushTimer.Reset(1 * time.Second)
			}
		case <-flushTimer.C:
			if len(batch) > 0 {
				batchCopy := make([]*T, len(batch))
				copy(batchCopy, batch)
				go u.uploadBatch(batchCopy)
				batch = batch[:0]
			}
			flushTimer.Reset(1 * time.Second)
		}
	}
}

func (u *Uploader[T]) uploadBatch(batch []*T) {
	switch u.uploadMode {
	case UploadModeDiagnostic:
		u.uploadDiagnosticBatch(batch)
	case UploadModeLog:
		u.uploadLogBatch(batch)
	}
}

// there's no need to do endpoint discovery, we can just hardcode the URLs
// it's guaranteed that if datadog-agent has Go DI it will also have the proxy upload endpoints

func (u *Uploader[T]) uploadLogBatch(batch []*T) {
	// TODO: find out if there are more efficient ways of sending logs to the backend
	// this is the way all other DI runtimes upload data
	url := fmt.Sprintf("http://%s:8126/debugger/v1/input", getAgentHost())
	body, _ := json.Marshal(batch)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Info("Failed to build request", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		log.Info("Error uploading log batch", err)
		return
	}
	defer resp.Body.Close()
	log.Info("HTTP", resp.StatusCode, url)
}

func (u *Uploader[T]) uploadDiagnosticBatch(batch []*T) {
	url := fmt.Sprintf("http://%s:8126/debugger/v1/diagnostics", getAgentHost())

	// Create a buffer to hold the multipart form data
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	diagnosticJSON, err := json.Marshal(batch)
	if err != nil {
		log.Info("Failed to marshal diagnostic batch", err, batch)
		return
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="event"; filename="event.json"`)
	header.Set("Content-Type", "application/json")
	fw, err := w.CreatePart(header)
	if err != nil {
		log.Info("Failed to create form file", err)
		return
	}

	// Write the JSON data to the form-data part
	if _, err = fw.Write(diagnosticJSON); err != nil {
		log.Info("Failed to write data to form file", err)
		return
	}

	// Close the multipart writer, otherwise the request will be missing the terminating boundary.
	w.Close()

	// Create a new request
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		log.Info("Failed to build request", err)
		return
	}

	// Set the content type to multipart/form-data and include the boundary
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := u.client.Do(req)
	if err != nil {
		log.Info("Error uploading diagnostic batch", err)
		return
	}
	defer resp.Body.Close()

	log.Info("HTTP", resp.StatusCode, url)
}

func getAgentHost() string {
	ddAgentHost := os.Getenv("DD_AGENT_HOST")
	if ddAgentHost == "" {
		ddAgentHost = "localhost"
	}
	return ddAgentHost
}
