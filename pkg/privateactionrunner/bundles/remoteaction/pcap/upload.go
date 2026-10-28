// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	pcapuploader "github.com/DataDog/datadog-agent/comp/networkpcap/uploader/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errUploaderUnavailable is returned when no uploader is wired in.
var errUploaderUnavailable = errors.New("packet capture uploader is not available")

// networkPcapEvent is the JSON metadata sent with the capture. The intake
// accepts a single attachment, so all metadata goes here.
type networkPcapEvent struct {
	DDSource  string `json:"ddsource"`
	CaptureID string `json:"capture_id"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"hostname"`
}

// uploadCapture sends the pcap file at pcapPath to the networkpcap intake. The
// uploader reopens the file on each attempt, so it is never held in memory.
// A capture cannot be re-taken, so failures are returned rather than dropped.
func uploadCapture(ctx context.Context, uploader pcapuploader.Component, hostname hostnameinterface.Component, pcapPath string, captureID string) error {
	if uploader == nil {
		return errUploaderUnavailable
	}

	// The capture ran on this host. The hostname comes from the core agent, so
	// it matches the host's other telemetry. A lookup failure is not fatal: the
	// capture is uploaded without one rather than lost.
	var hostnameValue string
	if hostname == nil {
		log.Warnf("networkpcap upload: no hostname provider for capture_id=%s", captureID)
	} else if h, err := hostname.Get(ctx); err != nil {
		log.Warnf("networkpcap upload: failed to resolve hostname for capture_id=%s: %s", captureID, err)
	} else {
		hostnameValue = h
	}

	event, err := json.Marshal(networkPcapEvent{
		DDSource:  "networkpcap",
		CaptureID: captureID,
		Timestamp: time.Now().UnixMilli(),
		Hostname:  hostnameValue,
	})
	if err != nil {
		return fmt.Errorf("marshaling event metadata: %w", err)
	}

	return uploader.Upload(ctx, pcapuploader.UploadRequest{
		Event: event,
		Attachments: []pcapuploader.Attachment{{
			Name:        "pcap",
			Filename:    "capture.pcap",
			ContentType: "application/octet-stream",
			Open: func() (io.ReadCloser, error) {
				return os.Open(pcapPath)
			},
		}},
	})
}
