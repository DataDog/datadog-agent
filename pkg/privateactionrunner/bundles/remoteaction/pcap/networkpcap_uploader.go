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
	"time"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

// defaultNetworkPcapIntakeHostPrefix is prepended to the configured Datadog
// site to build the default destination host for the "networkpcap" EVP
// attachment track's public intake. As of 2026-08-18 this hostname is not
// yet routed in either staging or prod (see
// ai-investigations/remote-pcap/notes.md and agent-uploader-implementation.md);
// network_pcap.logs_dd_url lets an operator override it in the meantime
// (e.g. "cws-intake.datad0g.com:443", which is routed and serves the same
// track lookup — the hostname only selects the intake door, the path below
// selects the track).
const defaultNetworkPcapIntakeHostPrefix = "pcap-intake."

const networkPcapPath = "/api/v2/networkpcap"

const (
	maxUploadAttempts  = 4
	uploadRetryBackoff = 2 * time.Second
)

// networkPcapUploader uploads a captured pcap file to the "networkpcap" EVP
// attachment track's public intake (POST /api/v2/networkpcap, DD-API-KEY
// auth), which is an upload_only track requiring multipart/form-data with a
// JSON "event" part and exactly one attachment part.
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
	return &networkPcapUploader{
		url:    fmt.Sprintf("https://%s%s", host, networkPcapPath),
		apiKey: cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// networkPcapEvent is the JSON metadata carried in the multipart "event"
// part. The track enforces max-attachment-count: 1, so all per-capture
// metadata must ride here rather than as a second attachment.
type networkPcapEvent struct {
	DDSource  string `json:"ddsource"`
	CaptureID string `json:"capture_id"`
	Timestamp int64  `json:"timestamp"`
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

func writeNetworkPcapAttachment(w *multipart.Writer, pcapBytes []byte) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="pcap"; filename="capture.pcap"`)
	h.Set("Content-Type", "application/octet-stream")

	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("creating pcap part: %w", err)
	}
	_, err = part.Write(pcapBytes)
	return err
}

// Upload sends pcapBytes to the networkpcap EVP track's public intake as a
// zstd-compressed, streamed multipart/form-data request (event → gzip-style
// codec → io.Pipe → request body, so the compressed payload is never
// materialized in memory — mirrors the secdump forwarder's pattern in
// pkg/security/security_profile/storage/backend/forwarder.go).
//
// Unlike secdump, this retries on transient failures and on 408 (the 30s
// intake-edge timeout, which is this track's real per-capture size ceiling,
// not max-content-size). It does not retry 413: the capture is already too
// large and retrying the same bytes cannot succeed. A pcap capture is not
// re-derivable, so failures are surfaced rather than silently dropped.
func (u *networkPcapUploader) Upload(ctx context.Context, pcapBytes []byte, captureID string) error {
	event := networkPcapEvent{
		DDSource:  "networkpcap",
		CaptureID: captureID,
		Timestamp: time.Now().UnixMilli(),
	}

	boundary := multipart.NewWriter(io.Discard).Boundary()
	contentType := "multipart/form-data; boundary=" + boundary

	newBody := func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			err := func() error {
				zw := zstd.NewWriter(pw)
				mw := multipart.NewWriter(zw)
				if err := mw.SetBoundary(boundary); err != nil {
					return err
				}
				if err := writeNetworkPcapEvent(mw, event); err != nil {
					return err
				}
				if err := writeNetworkPcapAttachment(mw, pcapBytes); err != nil {
					return err
				}
				if err := mw.Close(); err != nil {
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

		body, _ := newBody()
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
			lastErr = fmt.Errorf("networkpcap intake hit the 30s edge timeout (408); capture_id=%s may be too large for this host's uplink", captureID)
			continue
		case http.StatusRequestEntityTooLarge:
			return fmt.Errorf("networkpcap intake rejected capture as too large (413), capture_id=%s", captureID)
		default:
			return fmt.Errorf("networkpcap intake returned status %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("networkpcap upload failed after %d attempts: %w", maxUploadAttempts, lastErr)
}
