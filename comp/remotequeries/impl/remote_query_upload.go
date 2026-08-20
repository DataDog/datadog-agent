// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const (
	remoteQueryUploadDefaultFormat      = "csv"
	remoteQueryUploadDefaultCompression = "none"

	// Caps prevent int overflow and unreasonable upload sizes. chunk_bytes and max_bytes
	// are int32 on the wire, so the total upload is bounded well below int32 max.
	remoteQueryUploadMaxChunkBytes = 64 << 20 // 64 MiB
	remoteQueryUploadMaxTotalBytes = 1 << 30  // 1 GiB

	// Retry parameters for transient chunk PUT failures (transport errors, 408, 429, 5xx).
	remoteQueryUploadMaxRetries     = 4
	remoteQueryUploadInitialBackoff = 100 * time.Millisecond
	remoteQueryUploadMaxBackoff     = 5 * time.Second

	// Bounded JSON decode limit for the finalize response.
	remoteQueryUploadFinalizeResponseLimit = 1 << 20 // 1 MiB

	// remoteQueryUploadHTTPTimeout is the per-request hard cap for chunk PUTs and finalize/abort
	// POSTs. The stream context still drives cancellation; this is a safety net against a
	// silent stall.
	remoteQueryUploadHTTPTimeout = 60 * time.Second

	// remoteQueryUploadAbortTimeout is the best-effort cap for an abort POST. Abort must run
	// even after the stream context is cancelled, so it uses context.WithoutCancel plus this cap.
	remoteQueryUploadAbortTimeout = 5 * time.Second
)

var remoteQueryUploadIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// remoteQueryUploadAllowedBaseURL is the immutable, exact allowlisted Datadog intake
// service base URL permitted for chunked uploads. A configured base URL must match it
// EXACTLY (after validation): no wildcard subdomains, no arbitrary path/port. It is a
// const (not a mutable slice) so a leaked result_delivery.baseUrl can never exfiltrate the
// org API key/token to an attacker-controlled host, and tests cannot mutate it.
const remoteQueryUploadAllowedBaseURL = "https://dd.datad0g.com/api/intake/its-agent-intake" // POC staging intake

// validateRemoteQueryResultDelivery validates and normalizes an optional result-delivery
// instruction. It retains BaseURL and Token on the returned handle for the Agent Go upload
// relay; only the sanitized, non-secret fields are forwarded to the integration check
// (see remoteQueryExecuteRequest.internal). A nil delivery leaves the inline path unchanged.
func validateRemoteQueryResultDelivery(delivery *RemoteQueryResultDelivery, copyLimits *remoteQueryExecuteCopyLimits, copyFormat string) (*RemoteQueryResultDelivery, error) {
	if delivery == nil {
		return nil, nil
	}
	if delivery.Mode != RemoteQueryResultDeliveryModeChunkedUpload {
		return nil, fmt.Errorf("result_delivery.mode %q is not supported", delivery.Mode)
	}
	out := *delivery // copy so we can normalize without mutating the caller's value
	if out.Format == "" {
		out.Format = remoteQueryUploadDefaultFormat
	}
	if out.Compression == "" {
		out.Compression = remoteQueryUploadDefaultCompression
	}
	// Upload mode is CSV / no compression only.
	if out.Format != "csv" {
		return nil, errors.New("result_delivery.format must be csv")
	}
	if out.Compression != "none" {
		return nil, errors.New("result_delivery.compression must be none")
	}
	// The COPY stream format must match the upload format so emitted bytes and manifest agree.
	if copyFormat != "" && copyFormat != out.Format {
		return nil, errors.New("format must match result_delivery.format")
	}
	if out.UploadID == "" {
		return nil, errors.New("result_delivery.uploadId is required")
	}
	if !remoteQueryUploadIDPattern.MatchString(out.UploadID) {
		return nil, errors.New("result_delivery.uploadId contains invalid characters")
	}
	if out.BaseURL == "" {
		return nil, errors.New("result_delivery.baseUrl is required")
	}
	if err := validateUploadBaseURL(out.BaseURL); err != nil {
		return nil, err
	}
	if out.Token == "" {
		return nil, errors.New("result_delivery.token is required")
	}
	if out.ChunkBytes < 1 {
		return nil, errors.New("result_delivery.chunkBytes must be at least 1")
	}
	if out.ChunkBytes > remoteQueryUploadMaxChunkBytes {
		return nil, fmt.Errorf("result_delivery.chunkBytes must not exceed %d", remoteQueryUploadMaxChunkBytes)
	}
	if out.MaxBytes < 1 {
		return nil, errors.New("result_delivery.maxBytes must be at least 1")
	}
	if out.MaxBytes > remoteQueryUploadMaxTotalBytes {
		return nil, fmt.Errorf("result_delivery.maxBytes must not exceed %d", remoteQueryUploadMaxTotalBytes)
	}
	if out.ChunkBytes > out.MaxBytes {
		return nil, errors.New("result_delivery.chunkBytes must not exceed maxBytes")
	}
	// Upload caps must not widen the integration's configured COPY safety caps.
	if copyLimits != nil {
		if out.ChunkBytes > copyLimits.ChunkBytes {
			return nil, errors.New("result_delivery.chunkBytes must not exceed copyLimits.chunkBytes")
		}
		if out.MaxBytes > copyLimits.MaxBytes {
			return nil, errors.New("result_delivery.maxBytes must not exceed copyLimits.maxBytes")
		}
	}
	return &out, nil
}

// validateUploadBaseURL enforces an HTTPS Datadog intake service base URL that matches one of
// the exact allowlisted prefixes. It rejects wildcard subdomains, arbitrary paths/ports,
// query/userinfo/fragment, path traversal, and redirects (the HTTP client itself rejects
// redirects so auth headers never follow one). This prevents a leaked result_delivery.baseUrl
// from exfiltrating the org API key/token to an attacker-controlled host.
func validateUploadBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("result_delivery.baseUrl is malformed")
	}
	if u.Scheme != "https" {
		return errors.New("result_delivery.baseUrl must use https")
	}
	if u.User != nil {
		return errors.New("result_delivery.baseUrl must not contain userinfo")
	}
	if u.RawQuery != "" {
		return errors.New("result_delivery.baseUrl must not contain a query string")
	}
	if u.Fragment != "" {
		return errors.New("result_delivery.baseUrl must not contain a fragment")
	}
	if u.Host == "" {
		return errors.New("result_delivery.baseUrl must have a host")
	}
	// Default HTTPS port only: reject an explicit port other than the default 443.
	if port := u.Port(); port != "" && port != "443" {
		return errors.New("result_delivery.baseUrl must use the default https port")
	}
	if strings.Contains(u.Path, "..") || strings.Contains(u.Path, "//") {
		return errors.New("result_delivery.baseUrl path must not contain traversal")
	}
	if strings.HasSuffix(u.Path, "/") {
		return errors.New("result_delivery.baseUrl path must not end with a trailing slash")
	}
	// Require an EXACT match against the immutable allowlist (no suffix/wildcard matching,
	// no normalization that silently trims a trailing slash or collapses path segments).
	normalized := "https://" + u.Hostname() + u.Path
	if normalized == remoteQueryUploadAllowedBaseURL {
		return nil
	}
	return errors.New("result_delivery.baseUrl is not an allowlisted Datadog intake service")
}

// remoteQueryUploadTransport is the HTTP round-trip surface used by the upload relay. It is an
// interface so tests can inject canned responses without a live server.
type remoteQueryUploadTransport interface {
	roundTrip(ctx context.Context, method, urlStr string, headers map[string]string, body []byte) (status int, respBody []byte, err error)
}

// remoteQueryHTTPTransport is the production transport. It never logs request or response bodies.
type remoteQueryHTTPTransport struct {
	client *http.Client
}

func (t *remoteQueryHTTPTransport) roundTrip(ctx context.Context, method, urlStr string, headers map[string]string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	// Drain a bounded amount to allow connection reuse; never log the body.
	limited := io.LimitReader(resp.Body, remoteQueryUploadFinalizeResponseLimit+1)
	//reliabilint:ignore IGNORED_ERROR_RETURN reason="draining a bounded response body; a read error yields a partial body that the strict finalize decode rejects, and chunk PUTs ignore the body"
	respBody, _ := io.ReadAll(limited)
	return resp.StatusCode, respBody, nil
}

type remoteQueryUploadRelayConfig struct {
	mode     string
	uploadID string
	baseURL  string // validated, trailing slash trimmed
	//reliabilint:ignore SENSITIVE_FIELD_PLAIN_STRING reason="upload token is retained by the Agent Go relay and only used to set the Bearer header; never logged. Redacted[string] is unavailable in this repo"
	token string
	//reliabilint:ignore SENSITIVE_FIELD_PLAIN_STRING reason="org API key is retained by the Agent Go relay and only used to set the dd-api-key header; never logged. Redacted[string] is unavailable in this repo"
	apiKey      string
	chunkBytes  int32
	maxBytes    int32
	format      string
	compression string
}

// remoteQueryUploadRelay intercepts COPY stream data events and uploads bounded chunks
// directly to the Datadog intake service, suppressing bulk data from crossing the
// AgentSecure boundary. Only the compact upload receipt is surfaced downstream.
type remoteQueryUploadRelay struct {
	ctx        context.Context
	cfg        remoteQueryUploadRelayConfig
	transport  remoteQueryUploadTransport
	downstream func(check.RemoteQueryStreamEvent) error

	buffer     bytes.Buffer
	chunkIndex int32
	totalBytes int64
	totalRows  int64
	hash       hash.Hash

	finalized bool
	aborted   bool
	failed    error

	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

// newRemoteQueryUploadTransport builds the production HTTP transport. It uses the Agent's
// configured transport (httputils.CreateHTTPTransport) so proxy/TLS settings are honored,
// then wraps it in an http.Client that rejects ALL redirects (auth headers never follow)
// and caps each request at remoteQueryUploadHTTPTimeout.
func newRemoteQueryUploadTransport(cfg config.Component) remoteQueryUploadTransport {
	return &remoteQueryHTTPTransport{client: &http.Client{
		Transport: httputils.CreateHTTPTransport(cfg),
		Timeout:   remoteQueryUploadHTTPTimeout,
		// Reject ALL redirects so the dd-api-key and Bearer token never follow a redirect to an
		// attacker-controlled host. The validated base URL is the only destination we ever contact.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// newRemoteQueryUploadRelay builds the upload relay from a validated result-delivery handle.
// The org API key is retained here and never forwarded to the integration check. transport is
// injected (production uses the Agent-configured transport; tests inject a fake) so there is
// no unlocked package-global mutable state.
func newRemoteQueryUploadRelay(ctx context.Context, delivery *RemoteQueryResultDelivery, apiKey string, downstream func(check.RemoteQueryStreamEvent) error, transport remoteQueryUploadTransport) (*remoteQueryUploadRelay, error) {
	if delivery == nil || delivery.Mode != RemoteQueryResultDeliveryModeChunkedUpload {
		return nil, errors.New("result delivery is not configured for chunked upload")
	}
	if apiKey == "" {
		return nil, errors.New("remote query upload requires an org API key")
	}
	if transport == nil {
		return nil, errors.New("remote query upload requires an HTTP transport")
	}
	relay := &remoteQueryUploadRelay{
		ctx:            ctx,
		downstream:     downstream,
		hash:           sha256.New(),
		transport:      transport,
		maxRetries:     remoteQueryUploadMaxRetries,
		initialBackoff: remoteQueryUploadInitialBackoff,
		maxBackoff:     remoteQueryUploadMaxBackoff,
		cfg: remoteQueryUploadRelayConfig{
			mode:        delivery.Mode,
			uploadID:    delivery.UploadID,
			baseURL:     strings.TrimRight(delivery.BaseURL, "/"),
			token:       delivery.Token,
			apiKey:      apiKey,
			chunkBytes:  int32(delivery.ChunkBytes),
			maxBytes:    int32(delivery.MaxBytes),
			format:      delivery.Format,
			compression: delivery.Compression,
		},
	}
	return relay, nil
}

// emit is the callback the integration check calls. It intercepts data events (uploading
// them directly) and forwards only compact metadata/final/error events downstream.
// Once the relay reaches a terminal state (finalized or aborted) it fails closed: any
// further data or final event is rejected so a duplicate/late event can never re-upload or
// re-finalize after the receipt was already surfaced (or the session aborted).
func (r *remoteQueryUploadRelay) emit(event check.RemoteQueryStreamEvent) error {
	if r.failed != nil {
		return r.failed
	}
	if r.finalized || r.aborted {
		if r.failed == nil {
			r.failed = fmt.Errorf("remote query upload already %s", r.terminalState())
		}
		return r.failed
	}
	switch event.Type {
	case "metadata":
		return r.downstream(event)
	case "data":
		return r.handleData(event)
	case "final":
		return r.handleFinal(event)
	case "error":
		return r.handleError(event)
	default:
		return r.downstream(event)
	}
}

func (r *remoteQueryUploadRelay) terminalState() string {
	if r.finalized {
		return "finalized"
	}
	return "aborted"
}

func (r *remoteQueryUploadRelay) handleData(event check.RemoteQueryStreamEvent) error {
	if len(event.Payload) == 0 {
		return nil
	}
	if r.totalBytes+int64(len(event.Payload)) > int64(r.cfg.maxBytes) {
		r.failed = errors.New("remote query upload exceeded maxBytes")
		return r.failed
	}
	// Aggregate SHA-256 is tracked over raw chunk bodies in index order; writing every
	// byte in arrival order is equivalent to hashing the concatenation of chunk bodies.
	//reliabilint:ignore DISCARDED_ERROR_RETURN reason="hash.Hash.Write never returns a non-nil error per the io.Hash contract"
	r.hash.Write(event.Payload)
	r.totalBytes += int64(len(event.Payload))
	r.countRows(event.Payload)
	//reliabilint:ignore DISCARDED_ERROR_RETURN reason="bytes.Buffer.Write never returns a non-nil error"
	r.buffer.Write(event.Payload)
	for r.buffer.Len() >= int(r.cfg.chunkBytes) {
		if err := r.flushOneChunk(int(r.cfg.chunkBytes)); err != nil {
			r.failed = err
			return err
		}
	}
	return nil
}

func (r *remoteQueryUploadRelay) handleFinal(event check.RemoteQueryStreamEvent) error {
	// Flush the remaining buffer as the final chunk before finalizing.
	if r.buffer.Len() > 0 {
		if err := r.flushOneChunk(r.buffer.Len()); err != nil {
			r.failed = err
			r.abortOnce()
			return err
		}
	}
	receipt, err := r.postFinalize()
	if err != nil {
		r.failed = err
		r.abortOnce()
		return err
	}
	r.finalized = true
	return r.emitFinalReceipt(receipt)
}

func (r *remoteQueryUploadRelay) handleError(event check.RemoteQueryStreamEvent) error {
	r.abortOnce()
	return r.downstream(event)
}

// abortIfPending is called after the stream runner returns. If the relay never reached a
// final or error event (e.g. the runner returned an error mid-stream, or the context was
// cancelled), it POSTs an abort best effort.
func (r *remoteQueryUploadRelay) abortIfPending() {
	r.abortOnce()
}

// abortOnce marks the relay aborted and POSTs an abort exactly once. It sets aborted BEFORE
// the send so a concurrent handleError/abortIfPending cannot double-abort, and because
// intake abort is idempotent a racing second call is still safe. It is a no-op once finalized.
func (r *remoteQueryUploadRelay) abortOnce() {
	if r.finalized || r.aborted {
		return
	}
	r.aborted = true
	r.postAbort()
}

func (r *remoteQueryUploadRelay) flushOneChunk(size int) error {
	payload := append([]byte(nil), r.buffer.Next(size)...)
	if err := r.uploadChunk(r.chunkIndex, payload); err != nil {
		return err
	}
	r.chunkIndex++
	return nil
}

// uploadChunk PUTs a single raw chunk with bounded, context-aware exponential backoff. Only
// transient transport errors, 408, 429, and 5xx are retried; the same index and payload
// (and thus the same checksum) are re-sent on every attempt. Other 4xx are not retried.
func (r *remoteQueryUploadRelay) uploadChunk(index int32, payload []byte) error {
	chunkSum := sha256.Sum256(payload)
	headers := map[string]string{
		"Content-Type":  "application/octet-stream",
		"dd-api-key":    r.cfg.apiKey,
		"Authorization": "Bearer " + r.cfg.token,
		// Intake-required per-chunk digest and byte count (raw chunk body).
		"X-DD-Chunk-SHA256": hex.EncodeToString(chunkSum[:]),
		"X-DD-Chunk-Bytes":  strconv.Itoa(len(payload)),
		// Optional per-chunk row count; intake echoes it so totalRows matches end-to-end.
		"X-DD-Chunk-Rows": strconv.Itoa(countNewlines(payload)),
	}
	urlStr := r.chunkURL(index)
	backoff := r.initialBackoff
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		status, _, err := r.transport.roundTrip(r.ctx, http.MethodPut, urlStr, headers, payload)
		if err == nil && status >= 200 && status < 300 {
			return nil
		}
		if err == nil && !isTransientUploadStatus(status) {
			return fmt.Errorf("upload chunk %d rejected with status %d", index, status)
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("upload chunk %d returned status %d", index, status)
		}
		if attempt == r.maxRetries {
			break
		}
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > r.maxBackoff {
			backoff = r.maxBackoff
		}
	}
	return fmt.Errorf("upload chunk %d failed after %d attempts: %w", index, r.maxRetries+1, lastErr)
}

func isTransientUploadStatus(status int) bool {
	return status == http.StatusRequestTimeout || // 408
		status == http.StatusTooManyRequests || // 429
		status >= 500 // 5xx
}

// remoteQueryFinalizeRequest is the snake_case finalize request body posted to the Datadog intake
// finalize endpoint. The intake service speaks snake_case JSON.
type remoteQueryFinalizeRequest struct {
	Mode       string `json:"mode"`
	UploadID   string `json:"upload_id"`
	TotalBytes int64  `json:"total_bytes"`
	TotalRows  int64  `json:"total_rows"`
	ChunkCount int64  `json:"chunk_count"`
	SHA256     string `json:"sha256"`
}

type remoteQueryFinalizeResponse struct {
	Mode        string `json:"mode"`
	UploadID    string `json:"upload_id"`
	BucketName  string `json:"bucket_name"`
	ManifestKey string `json:"manifest_key"`
	TotalBytes  int64  `json:"total_bytes"`
	TotalRows   int64  `json:"total_rows"`
	ChunkCount  int64  `json:"chunk_count"`
	SHA256      string `json:"sha256"`
	Format      string `json:"format"`
	Compression string `json:"compression"`
	FinalizedAt string `json:"finalized_at"`
}

type remoteQueryUploadReceipt struct {
	Mode         string
	UploadID     string
	BucketName   string
	ManifestPath string
	TotalBytes   int64
	TotalRows    int64
	ChunkCount   int64
	SHA256       string
}

// postFinalize POSTs the finalize request and strictly decodes the bounded JSON response,
// validating uploadId, bucketName, manifestPath, totalBytes, totalRows, chunkCount, and the
// aggregate sha256 exactly and nonempty. The response body is never logged.
func (r *remoteQueryUploadRelay) postFinalize() (*remoteQueryUploadReceipt, error) {
	aggregate := hex.EncodeToString(r.hash.Sum(nil))
	body := remoteQueryFinalizeRequest{
		Mode:       r.cfg.mode,
		UploadID:   r.cfg.uploadID,
		TotalBytes: r.totalBytes,
		TotalRows:  r.totalRows,
		ChunkCount: int64(r.chunkIndex),
		SHA256:     aggregate,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"dd-api-key":    r.cfg.apiKey,
		"Authorization": "Bearer " + r.cfg.token,
	}
	// Finalize is idempotent on the intake side, so a lost response after the server durably
	// committed the manifest can recover with a bounded transient retry (transport/408/429/5xx).
	// Non-transient 4xx are not retried; the same ctx/backoff discipline as chunk PUTs applies.
	backoff := r.initialBackoff
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		receipt, status, err := r.postFinalizeOnce(headers, bodyBytes, aggregate)
		if err == nil {
			return receipt, nil
		}
		lastErr = err
		if !isTransientFinalizeError(status, err) {
			return nil, err
		}
		if attempt == r.maxRetries {
			break
		}
		select {
		case <-r.ctx.Done():
			return nil, r.ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > r.maxBackoff {
			backoff = r.maxBackoff
		}
	}
	return nil, fmt.Errorf("upload finalize failed after %d attempts: %w", r.maxRetries+1, lastErr)
}

// postFinalizeOnce performs a single finalize attempt: POST, bounded strict JSON decode (a
// trailing JSON value is rejected by requiring the second decode to return io.EOF), and
// exact validation of every canonical field. It returns the receipt on success, or the
// status/err so the caller can decide whether to retry.
func (r *remoteQueryUploadRelay) postFinalizeOnce(headers map[string]string, bodyBytes []byte, aggregate string) (*remoteQueryUploadReceipt, int, error) {
	status, respBody, err := r.transport.roundTrip(r.ctx, http.MethodPost, r.finalizeURL(), headers, bodyBytes)
	if err != nil {
		return nil, 0, fmt.Errorf("upload finalize failed: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, status, fmt.Errorf("upload finalize returned status %d", status)
	}
	if len(respBody) > remoteQueryUploadFinalizeResponseLimit {
		return nil, status, errors.New("upload finalize response exceeded size limit")
	}
	var resp remoteQueryFinalizeResponse
	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&resp); err != nil {
		return nil, status, fmt.Errorf("upload finalize response was invalid: %w", err)
	}
	// Reject trailing JSON: the response must be exactly one JSON object.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, status, errors.New("upload finalize response had trailing JSON")
	}
	if err := r.validateFinalizeResponse(resp, aggregate); err != nil {
		return nil, status, err
	}
	return &remoteQueryUploadReceipt{
		Mode:         r.cfg.mode,
		UploadID:     r.cfg.uploadID,
		BucketName:   resp.BucketName,
		ManifestPath: resp.ManifestKey,
		TotalBytes:   r.totalBytes,
		TotalRows:    r.totalRows,
		ChunkCount:   int64(r.chunkIndex),
		SHA256:       aggregate,
	}, status, nil
}

func isTransientFinalizeError(status int, err error) bool {
	if err != nil && status == 0 {
		return true // transport error
	}
	return isTransientUploadStatus(status)
}

// validateFinalizeResponse strictly validates the intake finalize response fields exactly and
// nonempty; the aggregate sha256 must equal the relay's locally computed hash. The
// intake-echoed mode/format/compression must match the validated delivery (csv/none).
func (r *remoteQueryUploadRelay) validateFinalizeResponse(resp remoteQueryFinalizeResponse, aggregate string) error {
	if resp.UploadID != r.cfg.uploadID {
		return errors.New("upload finalize response upload_id mismatch")
	}
	if resp.Mode != r.cfg.mode {
		return errors.New("upload finalize response mode mismatch")
	}
	if resp.BucketName == "" {
		return errors.New("upload finalize response missing bucket_name")
	}
	if resp.ManifestKey == "" {
		return errors.New("upload finalize response missing manifest_key")
	}
	if resp.TotalBytes != r.totalBytes {
		return errors.New("upload finalize response total_bytes mismatch")
	}
	if resp.TotalRows != r.totalRows {
		return errors.New("upload finalize response total_rows mismatch")
	}
	if resp.ChunkCount != int64(r.chunkIndex) {
		return errors.New("upload finalize response chunk_count mismatch")
	}
	if resp.SHA256 == "" || resp.SHA256 != aggregate {
		return errors.New("upload finalize response sha256 mismatch")
	}
	if resp.Format != r.cfg.format {
		return errors.New("upload finalize response format mismatch")
	}
	if resp.Compression != r.cfg.compression {
		return errors.New("upload finalize response compression mismatch")
	}
	return nil
}

// postAbort POSTs an abort best effort; errors are ignored and never logged with secrets. It
// uses context.WithoutCancel plus a short timeout so the abort can still be delivered after
// the stream context is cancelled (e.g. on cancellation or a mid-stream runner error).
func (r *remoteQueryUploadRelay) postAbort() {
	headers := map[string]string{
		"Content-Type":  "application/json",
		"dd-api-key":    r.cfg.apiKey,
		"Authorization": "Bearer " + r.cfg.token,
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.ctx), remoteQueryUploadAbortTimeout)
	defer cancel()
	//reliabilint:ignore IGNORED_ERROR_RETURN reason="abort is best-effort by contract; errors are intentionally ignored and never logged with secrets"
	_, _, _ = r.transport.roundTrip(ctx, http.MethodPost, r.abortURL(), headers, []byte("{}"))
}

// emitFinalReceipt surfaces only the compact final metadata (the upload receipt) downstream,
// replacing the integration's provisional receipt with the Agent-enriched, validated receipt.
func (r *remoteQueryUploadRelay) emitFinalReceipt(receipt *remoteQueryUploadReceipt) error {
	metadata := map[string]interface{}{
		"status":         "SUCCEEDED",
		"bytes_emitted":  receipt.TotalBytes,
		"chunks_emitted": receipt.ChunkCount,
		"upload_receipt": map[string]interface{}{
			"mode":         receipt.Mode,
			"uploadId":     receipt.UploadID,
			"bucketName":   receipt.BucketName,
			"manifestPath": receipt.ManifestPath,
			"totalBytes":   receipt.TotalBytes,
			"totalRows":    receipt.TotalRows,
			"chunkCount":   receipt.ChunkCount,
			"sha256":       receipt.SHA256,
		},
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return r.downstream(check.RemoteQueryStreamEvent{
		Type:         "final",
		MetadataJSON: string(metaJSON),
	})
}

// countRows is a POC approximation of CSV row count via newline bytes. The intake echoes
// the relay's value, so the aggregate is consistent end-to-end.
// countNewlines returns the number of '\n' bytes in payload. It is a POC approximation of CSV row
// count; the intake echoes the relay's value, so the aggregate is consistent end-to-end.
func countNewlines(payload []byte) int {
	rows := 0
	for _, b := range payload {
		if b == '\n' {
			rows++
		}
	}
	return rows
}

func (r *remoteQueryUploadRelay) countRows(payload []byte) {
	r.totalRows += int64(countNewlines(payload))
}

func (r *remoteQueryUploadRelay) chunkURL(index int32) string {
	return r.cfg.baseURL + "/uploads/" + r.cfg.uploadID + "/chunks/" + strconv.Itoa(int(index))
}

func (r *remoteQueryUploadRelay) finalizeURL() string {
	return r.cfg.baseURL + "/uploads/" + r.cfg.uploadID + "/finalize"
}

func (r *remoteQueryUploadRelay) abortURL() string {
	return r.cfg.baseURL + "/uploads/" + r.cfg.uploadID + "/abort"
}
