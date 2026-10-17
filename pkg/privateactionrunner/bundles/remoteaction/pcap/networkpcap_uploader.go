// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

// defaultNetworkPcapIntakeHostPrefix is prepended to the Datadog site to build
// the intake host; network_pcap.logs_dd_url overrides it.
const defaultNetworkPcapIntakeHostPrefix = "cnm-intake."

const networkPcapPath = "/api/v2/networkpcap"

const (
	maxUploadAttempts  = 4
	uploadRetryBackoff = 2 * time.Second
)

// networkPcapUploader uploads a captured pcap file to the networkpcap intake
// as multipart/form-data: a JSON "event" part and one pcap attachment part.
type networkPcapUploader struct {
	url        string
	apiKey     string
	httpClient *http.Client
}

func newNetworkPcapUploader(cfg *config.Config) *networkPcapUploader {
	host := cfg.NetworkPcapLogsDDURL
	if host == "" {
		host = defaultNetworkPcapIntakeHostPrefix + cfg.DatadogSite
	}
	// Reuse the Agent's transport so proxy and TLS settings apply; the client
	// is copied so the upload timeout does not leak into other callers.
	httpClient := &http.Client{Timeout: 60 * time.Second}
	if cfg.AgentHTTPClient != nil {
		httpClient.Transport = cfg.AgentHTTPClient.Transport
	}
	return &networkPcapUploader{
		url:        fmt.Sprintf("https://%s%s", host, networkPcapPath),
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
	}
}

// networkPcapEvent is the JSON metadata carried in the multipart "event"
// part. The intake accepts a single attachment, so all metadata goes here.
type networkPcapEvent struct {
	DDSource  string `json:"ddsource"`
	CaptureID string `json:"capture_id"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"hostname"`
}

func writeNetworkPcapEvent(w *multipart.Writer, event networkPcapEvent) error {
	h := make(textproto.MIMEHeader)
	// filename="" is required: the intake decoder only treats a part as a
	// file upload (and therefore accepts it at all) when it carries a
	// filename attribute, even an empty one. Without it: 400 "File event
	// not found in the request".
	h.Set("Content-Disposition", `form-data; name="event"; filename=""`)
	h.Set("Content-Type", "application/json")

	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("creating event part: %w", err)
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event metadata: %w", err)
	}
	_, err = part.Write(eventJSON)
	return err
}

func writeNetworkPcapAttachment(w *multipart.Writer, pcap io.Reader) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="pcap"; filename="capture.pcap"`)
	h.Set("Content-Type", "application/octet-stream")

	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("creating pcap part: %w", err)
	}
	_, err = io.Copy(part, pcap)
	return err
}

// writeMultipart writes the event and the pcap attachment to w as a
// multipart body with the given boundary.
func writeMultipart(w io.Writer, boundary string, event networkPcapEvent, pcap io.Reader) error {
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return err
	}
	if err := writeNetworkPcapEvent(mw, event); err != nil {
		return err
	}
	if err := writeNetworkPcapAttachment(mw, pcap); err != nil {
		return err
	}
	return mw.Close()
}

// Upload sends the pcap file at pcapPath as a zstd-compressed, streamed
// multipart request. Each attempt reads the file again, so neither the capture
// nor the compressed payload is held in memory.
//
// It retries on transport errors, 408, 429 and 5xx. It does not retry 413:
// resending the same bytes cannot succeed. A capture cannot be re-taken, so
// failures are returned rather than dropped.
func (u *networkPcapUploader) Upload(ctx context.Context, pcapPath string, captureID string) error {
	// The capture ran on this host. A hostname lookup failure is not fatal:
	// the capture is uploaded without one rather than lost.
	hostnameValue, err := hostname.Get(ctx)
	if err != nil {
		log.Warnf("networkpcap upload: failed to resolve hostname for capture_id=%s: %s", captureID, err)
	}

	event := networkPcapEvent{
		DDSource:  "networkpcap",
		CaptureID: captureID,
		Timestamp: time.Now().UnixMilli(),
		Hostname:  hostnameValue,
	}

	boundary := multipart.NewWriter(io.Discard).Boundary()
	contentType := "multipart/form-data; boundary=" + boundary

	newBody := func() (io.ReadCloser, error) {
		f, err := os.Open(pcapPath)
		if err != nil {
			return nil, fmt.Errorf("opening captured pcap file: %w", err)
		}
		pr, pw := io.Pipe()
		go func() {
			defer f.Close()
			err := func() error {
				zw, err := zstd.NewWriter(pw)
				if err != nil {
					return err
				}
				if err := writeMultipart(zw, boundary, event, f); err != nil {
					// Close frees the compressor even when the body was
					// abandoned, e.g. the server answered before reading it.
					_ = zw.Close()
					return err
				}
				return zw.Close()
			}()
			_ = pw.CloseWithError(err)
		}()
		return pr, nil
	}

	var lastErr error
	for attempt := 0; attempt < maxUploadAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(uploadRetryBackoff * time.Duration(attempt)):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		body, err := newBody()
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url, body)
		if err != nil {
			_ = body.Close()
			return fmt.Errorf("building request: %w", err)
		}
		req.GetBody = newBody
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Content-Encoding", "zstd")
		req.Header.Set("DD-API-KEY", u.apiKey)

		resp, err := u.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("sending request: %w", err)
			continue
		}
		_ = resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusAccepted:
			return nil
		case http.StatusRequestTimeout:
			lastErr = fmt.Errorf("networkpcap intake timed out (408) receiving capture_id=%s", captureID)
			continue
		case http.StatusRequestEntityTooLarge:
			return fmt.Errorf("networkpcap intake rejected capture as too large (413), capture_id=%s", captureID)
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			lastErr = fmt.Errorf("networkpcap intake returned status %d", resp.StatusCode)
			continue
		default:
			return fmt.Errorf("networkpcap intake returned status %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("networkpcap upload failed after %d attempts: %w", maxUploadAttempts, lastErr)
}
