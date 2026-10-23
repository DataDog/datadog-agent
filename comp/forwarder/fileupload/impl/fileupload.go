// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fileuploadimpl implements the file upload forwarder component.
package fileuploadimpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"sync"
	"time"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/hostport"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

const (
	defaultMaxAttempts  = 4
	defaultRetryBackoff = 2 * time.Second
	// defaultAttemptTimeout bounds a single request, body included.
	defaultAttemptTimeout = 60 * time.Second
)

// Requires defines the component's dependencies.
type Requires struct {
	Config configcomp.Component
}

// Provides defines the component's output.
type Provides struct {
	Comp fileupload.Component
}

// NewComponent creates a new file upload forwarder component.
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: newUploader(reqs.Config, getTracks())}
}

type uploader struct {
	cfg          model.Reader
	tracks       map[fileupload.TrackType]trackDesc
	httpClient   *http.Client
	maxAttempts  int
	retryBackoff time.Duration

	mu        sync.Mutex
	endpoints map[fileupload.TrackType]*logsconfig.Endpoints
}

func newUploader(cfg model.Reader, descs []trackDesc) *uploader {
	tracks := make(map[fileupload.TrackType]trackDesc, len(descs))
	for _, desc := range descs {
		tracks[desc.trackType] = desc
	}
	return &uploader{
		cfg:    cfg,
		tracks: tracks,
		// The Agent transport applies the proxy and TLS settings.
		httpClient: &http.Client{
			Timeout:   defaultAttemptTimeout,
			Transport: httputils.CreateHTTPTransport(cfg),
		},
		maxAttempts:  defaultMaxAttempts,
		retryBackoff: defaultRetryBackoff,
		endpoints:    make(map[fileupload.TrackType]*logsconfig.Endpoints),
	}
}

// getEndpoints resolves the endpoints of a track once, the same way the event
// platform forwarder does: <prefix>logs_dd_url / dd_url override the main
// endpoint, <prefix>additional_endpoints adds more, and the MRF endpoint is
// added when multi_region_failover is enabled. Each endpoint carries its own
// API key, which follows runtime config updates.
func (u *uploader) getEndpoints(track fileupload.TrackType) (*logsconfig.Endpoints, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if endpoints, ok := u.endpoints[track]; ok {
		return endpoints, nil
	}
	desc, ok := u.tracks[track]
	if !ok {
		return nil, fmt.Errorf("unknown file upload track %q", track)
	}
	configKeys := logsconfig.NewLogsConfigKeys(desc.endpointsConfigPrefix, u.cfg)
	endpoints, err := logsconfig.BuildHTTPEndpointsWithConfig(
		u.cfg,
		configKeys,
		desc.hostnameEndpointPrefix,
		logsconfig.IntakeTrackType(desc.trackType),
		logsconfig.DefaultIntakeProtocol,
		logsconfig.DefaultIntakeOrigin,
	)
	if err != nil {
		return nil, fmt.Errorf("building endpoints for track %s: %w", track, err)
	}
	u.endpoints[track] = endpoints
	return endpoints, nil
}

// Upload sends req to every endpoint of its track concurrently. Failures on
// reliable endpoints are returned; failures on unreliable ones are logged.
func (u *uploader) Upload(ctx context.Context, req fileupload.UploadRequest) error {
	endpoints, err := u.getEndpoints(req.Track)
	if err != nil {
		return err
	}

	// The MRF endpoint only receives data while a failover is active, like
	// every other sender on the logs/EvP intake.
	mrfActive := u.cfg.GetBool("multi_region_failover.enabled") && u.cfg.GetBool("multi_region_failover.failover_logs")

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for i := range endpoints.Endpoints {
		endpoint := &endpoints.Endpoints[i]
		if endpoint.IsMRF && !mrfActive {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			endpointURL := buildURL(endpoint, req.Track)
			if err := u.sendWithRetries(ctx, endpoint, endpointURL, req); err != nil {
				if !endpoint.IsReliable() {
					log.Warnf("file upload to unreliable endpoint %s failed: %v", endpointURL, err)
					return
				}
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", endpointURL, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// sendWithRetries retries transport errors, 408, 429 and 5xx. 413 is not
// retried: resending the same bytes cannot succeed.
func (u *uploader) sendWithRetries(ctx context.Context, endpoint *logsconfig.Endpoint, endpointURL string, req fileupload.UploadRequest) error {
	var lastErr error
	for attempt := 0; attempt < u.maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(u.retryBackoff * time.Duration(attempt)):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		retryable, err := u.send(ctx, endpoint, endpointURL, req)
		if err == nil {
			return nil
		}
		if !retryable {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("upload failed after %d attempts: %w", u.maxAttempts, lastErr)
}

// send makes one attempt. It reports whether a failure is worth retrying.
func (u *uploader) send(ctx context.Context, endpoint *logsconfig.Endpoint, endpointURL string, req fileupload.UploadRequest) (bool, error) {
	apiKey, ready := endpoint.GetAPIKeyIfReady()
	if !ready || apiKey == "" {
		return false, errors.New("no API key available for this endpoint")
	}

	boundary := multipart.NewWriter(io.Discard).Boundary()
	newBody := func() (io.ReadCloser, error) {
		return newMultipartBody(boundary, req)
	}

	body, err := newBody()
	if err != nil {
		// The attachments cannot be read; retrying will not help.
		return false, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, body)
	if err != nil {
		_ = body.Close()
		return false, fmt.Errorf("building request: %w", err)
	}
	httpReq.GetBody = newBody
	httpReq.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	httpReq.Header.Set("Content-Encoding", "zstd")
	httpReq.Header.Set("DD-API-KEY", apiKey)
	for k, v := range endpoint.ExtraHTTPHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := u.httpClient.Do(httpReq)
	if err != nil {
		return true, fmt.Errorf("sending request: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()

	switch code := resp.StatusCode; {
	case code >= 200 && code < 300:
		return false, nil
	case code == http.StatusRequestEntityTooLarge:
		return false, fmt.Errorf("intake rejected the upload as too large (%d)", code)
	case code == http.StatusRequestTimeout, code == http.StatusTooManyRequests, code >= 500:
		return true, fmt.Errorf("intake returned status %d", code)
	default:
		return false, fmt.Errorf("intake returned status %d", code)
	}
}

// newMultipartBody opens the attachments and returns a reader that streams
// the zstd-compressed multipart body: the event part, then each attachment.
// Opening up front surfaces unreadable attachments before anything is sent.
func newMultipartBody(boundary string, req fileupload.UploadRequest) (io.ReadCloser, error) {
	readers := make([]io.ReadCloser, 0, len(req.Attachments))
	closeAll := func() {
		for _, r := range readers {
			_ = r.Close()
		}
	}
	for _, a := range req.Attachments {
		if a.Open == nil {
			closeAll()
			return nil, fmt.Errorf("attachment %q has no content", a.Name)
		}
		r, err := a.Open()
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("opening attachment %q: %w", a.Name, err)
		}
		readers = append(readers, r)
	}

	pr, pw := io.Pipe()
	go func() {
		defer closeAll()
		err := func() error {
			zw, err := zstd.NewWriter(pw)
			if err != nil {
				return err
			}
			if err := writeMultipart(zw, boundary, req, readers); err != nil {
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

func writeMultipart(w io.Writer, boundary string, req fileupload.UploadRequest, readers []io.ReadCloser) error {
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return err
	}

	h := make(textproto.MIMEHeader)
	// filename="" is required: the intake only accepts a part as a file
	// upload when it carries a filename attribute, even an empty one.
	// Without it: 400 "File event not found in the request".
	h.Set("Content-Disposition", `form-data; name="event"; filename=""`)
	h.Set("Content-Type", "application/json")
	part, err := mw.CreatePart(h)
	if err != nil {
		return fmt.Errorf("creating event part: %w", err)
	}
	if _, err := part.Write(req.Event); err != nil {
		return fmt.Errorf("writing event part: %w", err)
	}

	for i, a := range req.Attachments {
		contentType := a.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, a.Name, a.Filename))
		h.Set("Content-Type", contentType)
		part, err := mw.CreatePart(h)
		if err != nil {
			return fmt.Errorf("creating attachment part %q: %w", a.Name, err)
		}
		if _, err := io.Copy(part, readers[i]); err != nil {
			return fmt.Errorf("writing attachment part %q: %w", a.Name, err)
		}
	}
	return mw.Close()
}

// buildURL returns the upload URL of a track on an endpoint. Uploads always
// use the v2 intake API, whatever <prefix>use_v2_api says.
func buildURL(endpoint *logsconfig.Endpoint, track fileupload.TrackType) string {
	scheme := "http"
	if endpoint.UseSSL() {
		scheme = "https"
	}
	host := endpoint.Host
	if endpoint.Port != 0 {
		host = hostport.Join(endpoint.Host, strconv.Itoa(endpoint.Port))
	}
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   endpoint.PathPrefix + "/api/v2/" + string(track),
	}
	return u.String()
}
