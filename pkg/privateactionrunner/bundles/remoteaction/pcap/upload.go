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

	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// networkPcapEvent is the JSON metadata sent with the capture. The intake
// accepts a single attachment, so all metadata goes here.
type networkPcapEvent struct {
	DDSource  string `json:"ddsource"`
	CaptureID string `json:"capture_id"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"hostname"`
}

// uploadCapture sends the pcap file at pcapPath to the networkpcap track. The
// forwarder reopens the file on each attempt, so it is never held in memory.
// A capture cannot be re-taken, so failures are returned rather than dropped.
func uploadCapture(ctx context.Context, uploader fileupload.Component, pcapPath string, captureID string) error {
	if uploader == nil {
		return errors.New("file upload forwarder is not available")
	}

	// The capture ran on this host. A hostname lookup failure is not fatal:
	// the capture is uploaded without one rather than lost.
	hostnameValue, err := hostname.Get(ctx)
	if err != nil {
		log.Warnf("networkpcap upload: failed to resolve hostname for capture_id=%s: %s", captureID, err)
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

	return uploader.Upload(ctx, fileupload.UploadRequest{
		Track: fileupload.TrackNetworkPcap,
		Event: event,
		Attachments: []fileupload.Attachment{{
			Name:        "pcap",
			Filename:    "capture.pcap",
			ContentType: "application/octet-stream",
			Open: func() (io.ReadCloser, error) {
				return os.Open(pcapPath)
			},
		}},
	})
}
