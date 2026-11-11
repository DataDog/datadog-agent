// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"errors"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	pcapuploader "github.com/DataDog/datadog-agent/comp/networkpcap/uploader/def"
)

// Outcome reasons of a runCapture action. The set is closed so the metric
// keeps a bounded number of contexts.
const (
	reasonSuccess             = "success"
	reasonInvalidInput        = "invalid_input"
	reasonSocketUnreachable   = "socket_unreachable"
	reasonBadRequest          = "bad_request"
	reasonBusy                = "busy"
	reasonCaptureError        = "capture_error"
	reasonUploaderUnavailable = "uploader_unavailable"
	reasonUploadTooLarge      = "upload_too_large"
	reasonUploadFailed        = "upload_failed"
)

// System-probe sockets a capture can be requested on.
const (
	// socketCapture is the capture-only socket (packet_capture.socket).
	socketCapture = "capture"
	// socketMain is the main system-probe socket, used when no capture-only
	// socket is configured.
	socketMain = "main"
	// socketNone means no socket was dialed.
	socketNone = "none"
)

// captureOutcomes counts runCapture actions, once each, by outcome and by the
// system-probe socket used.
var captureOutcomes = telemetryimpl.GetCompatComponent().NewCounter(
	"remote_pcap", "capture_outcome", []string{"reason", "socket"},
	"Packet capture actions by outcome reason and system-probe socket used",
)

// reasonError tags an error with the outcome reason it maps to.
type reasonError struct {
	reason string
	err    error
}

func (e *reasonError) Error() string { return e.err.Error() }
func (e *reasonError) Unwrap() error { return e.err }

func withReason(reason string, err error) error {
	return &reasonError{reason: reason, err: err}
}

// captureFailureReason maps an error returned by a captureTrigger to an
// outcome reason. Errors that carry no reason are capture errors.
func captureFailureReason(err error) string {
	var re *reasonError
	if errors.As(err, &re) {
		return re.reason
	}
	return reasonCaptureError
}

// uploadFailureReason maps an error returned by uploadCapture to an outcome
// reason.
func uploadFailureReason(err error) string {
	switch {
	case errors.Is(err, errUploaderUnavailable):
		return reasonUploaderUnavailable
	case errors.Is(err, pcapuploader.ErrPayloadTooLarge):
		return reasonUploadTooLarge
	default:
		return reasonUploadFailed
	}
}
