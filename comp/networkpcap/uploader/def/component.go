// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package uploader sends network packet captures to the networkpcap intake as streamed multipart uploads
package uploader

// team: cloud-network-monitoring

import (
	"context"
	"io"
)

// Component uploads network packet captures to the networkpcap intake.
//
// Unlike the event platform forwarder, which batches small JSON events, this
// component sends one request per upload and streams the attachments from
// their source, so captures of any size can be sent without being held in
// memory.
type Component interface {
	// Upload sends req to every configured networkpcap endpoint and blocks
	// until it is accepted or has failed. It returns an error when a reliable
	// endpoint did not accept the upload.
	Upload(ctx context.Context, req UploadRequest) error
}

// UploadRequest is a single packet capture upload: a JSON event and its
// attachments, sent as one multipart/form-data body.
type UploadRequest struct {
	// Event is the JSON metadata sent as the "event" part.
	Event []byte
	// Attachments are sent as file parts after the event, in order.
	Attachments []Attachment
}

// Attachment is a file part of an upload.
type Attachment struct {
	// Name is the form field name of the part, e.g. "pcap".
	Name string
	// Filename is the filename attribute of the part, e.g. "capture.pcap".
	Filename string
	// ContentType is the content type of the part. Empty means
	// "application/octet-stream".
	ContentType string
	// Open returns a fresh reader over the attachment content. It is called
	// once per attempt and per endpoint, possibly concurrently, so retries
	// resend the whole content without buffering it.
	Open func() (io.ReadCloser, error)
}
