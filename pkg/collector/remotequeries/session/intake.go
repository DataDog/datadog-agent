// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Bounds of intake requests; the integration specs list them as limits.
const (
	ConnectTimeout     = 10 * time.Second
	SendStallTimeout   = 30 * time.Second
	PageAttemptTimeout = 6 * time.Minute
	OtherAttemptTime   = 30 * time.Second
	AbortTimeout       = 10 * time.Second
	MaxAttempts        = 5
	InitialBackoff     = 100 * time.Millisecond
	MaxBackoff         = 5 * time.Second

	maxResponseBytes = 64 * 1024
	// maxAbortMessageBytes is how much of an abort message intake keeps.
	maxAbortMessageBytes = 1024
)

// IntakeError is a failed intake request. It carries only a stable code: intake's
// response messages are never propagated.
type IntakeError struct{ Code string }

func (e *IntakeError) Error() string { return "intake request failed: " + e.Code }

// IsIntakeCode reports whether err is an intake error with the code.
func IsIntakeCode(err error, code string) bool {
	var ie *IntakeError
	return errors.As(err, &ie) && ie.Code == code
}

var intakeErrorCodes = map[string]bool{
	"invalid_argument": true, "unauthenticated": true, "not_found": true, "already_exists": true,
	"failed_precondition": true, "gone": true, "payload_too_large": true, "result_file_too_large": true,
	"scan_failed": true, "unsupported_encoding": true, "storage_access_denied": true,
	"unavailable": true, "internal": true,
}

// NewHTTPClient returns the client for intake requests: it follows the Agent's proxy
// configuration, always verifies intake's certificate, never follows a redirect, and
// bounds connecting. Call the returned function when the run ends.
func NewHTTPClient(cfg pkgconfigmodel.Reader) (*http.Client, func()) {
	transport := httputils.CreateHTTPTransport(cfg, func(t *http.Transport) {
		t.DialContext = (&net.Dialer{Timeout: ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext
	})
	// Every request carries the API key: verify intake's certificate even when the
	// Agent is configured to skip verification for other traffic.
	if transport.TLSClientConfig != nil {
		transport.TLSClientConfig.InsecureSkipVerify = false
	}
	if transport.TLSHandshakeTimeout <= 0 || transport.TLSHandshakeTimeout > ConnectTimeout {
		transport.TLSHandshakeTimeout = ConnectTimeout
	}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return client, transport.CloseIdleConnections
}

// Client talks to one upload session. It has no configurable destination and no
// credential other than the Agent's API key.
type Client struct {
	http   *http.Client
	base   string
	apiKey string
	trace  *Trace
	// Set once intake refuses gzip: the rest of the run sends pages uncompressed.
	identityOnly bool
	// sleep and stall are replaced in tests.
	sleep func(context.Context, time.Duration) error
	stall time.Duration
}

// NewClient returns a client for the delivery's upload session.
func NewClient(httpClient *http.Client, d *Delivery, apiKey string, trace *Trace) *Client {
	return &Client{
		http:   httpClient,
		base:   strings.TrimRight(d.BaseURL, "/") + "/uploads/" + url.PathEscape(d.UploadID),
		apiKey: apiKey,
		trace:  trace,
		sleep:  sleepContext,
		stall:  SendStallTimeout,
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type intakeRequest struct {
	method      string
	suffix      string
	contentType string
	body        []byte
	headers     map[string]string
	timeout     time.Duration
	// identity switches a compressed page to its uncompressed form; nil when the body is
	// not compressed.
	identity func(*intakeRequest)
}

// RegisterDescriptor registers the run's descriptor.
func (c *Client) RegisterDescriptor(ctx context.Context, body []byte) error {
	return c.do(ctx, &intakeRequest{method: http.MethodPost, suffix: "/descriptor", contentType: "application/json", body: body, timeout: OtherAttemptTime})
}

// PageUpload describes an acknowledged page upload, for logging.
type PageUpload struct {
	SentBytes      int
	Encoding       string
	Compress       time.Duration
	Upload         time.Duration
	GzipNotSmaller bool
}

// PutPage uploads one page. The body is gzip-compressed when that makes it smaller and
// intake has not refused the encoding; every attempt sends the same bytes.
func (c *Client) PutPage(ctx context.Context, index int, page []byte, rows, offset int64) (PageUpload, error) {
	headers := map[string]string{
		"X-DD-Source-Page-Rows":  strconv.FormatInt(rows, 10),
		"X-DD-Record-Offset":     strconv.FormatInt(offset, 10),
		"X-DD-Source-Page-Bytes": strconv.Itoa(len(page)),
	}
	req := &intakeRequest{method: http.MethodPut, suffix: "/pages/" + strconv.Itoa(index), contentType: "text/csv", body: page, headers: headers, timeout: PageAttemptTimeout}
	result := PageUpload{Encoding: "identity"}
	if !c.identityOnly && len(page) > 0 {
		start := time.Now()
		compressed := gzipBytes(page)
		result.Compress = time.Since(start)
		if len(compressed) < len(page) {
			req.body = compressed
			req.headers = map[string]string{"Content-Encoding": "gzip"}
			for k, v := range headers {
				req.headers[k] = v
			}
			req.identity = func(r *intakeRequest) {
				c.identityOnly = true
				r.body, r.headers, r.identity = page, headers, nil
			}
		} else {
			result.GzipNotSmaller = true
		}
	}
	start := time.Now()
	err := c.do(ctx, req)
	result.Upload = time.Since(start)
	result.SentBytes = len(req.body)
	if req.identity != nil {
		result.Encoding = "gzip"
	}
	return result, err
}

// gzipBytes compresses at the fastest level. The gzip header carries no name or
// modification time, so the same page always compresses to the same bytes.
func gzipBytes(page []byte) []byte {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	_, _ = w.Write(page)
	_ = w.Close()
	return buf.Bytes()
}

// Finalize finishes the upload with the number of accepted pages.
func (c *Client) Finalize(ctx context.Context, pages int) error {
	body, _ := json.Marshal(map[string]int{"expected_page_count": pages})
	return c.do(ctx, &intakeRequest{method: http.MethodPost, suffix: "/finalize", contentType: "application/json", body: body, timeout: OtherAttemptTime})
}

// Abort stops the upload and records why. It has its own time budget and does not
// depend on the run's context, which is usually over by then.
func (c *Client) Abort(reason *Failure) error {
	ctx, cancel := context.WithTimeout(context.Background(), AbortTimeout)
	defer cancel()
	var body []byte
	if reason != nil {
		body, _ = json.Marshal(map[string]map[string]string{"error": {"code": reason.Code, "message": truncateUTF8(reason.Message, maxAbortMessageBytes)}})
	}
	return c.do(ctx, &intakeRequest{method: http.MethodPost, suffix: "/abort", contentType: "application/json", body: body, timeout: AbortTimeout})
}

func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// do sends one request, retrying capacity and transient failures with bounded
// exponential backoff and jitter.
func (c *Client) do(ctx context.Context, req *intakeRequest) error {
	failures := 0
	delay := InitialBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, code, err := c.attempt(ctx, req)
		switch {
		case err == nil && status == http.StatusNoContent:
			return nil
		case err == nil && status == http.StatusUnsupportedMediaType && code == "unsupported_encoding" && req.identity != nil:
			// Not an attempt: resend at once, uncompressed.
			req.identity(req)
			continue
		case err == nil && status == http.StatusServiceUnavailable && code == "unavailable":
			// Capacity: retry until the context ends, without counting a failure.
		case err == nil && (status != http.StatusInternalServerError || code != "internal"):
			if status >= 300 && status < 400 {
				return &IntakeError{Code: "redirect"}
			}
			return &IntakeError{Code: code}
		default:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			if failures >= MaxAttempts {
				if err != nil {
					return &IntakeError{Code: "transport_error"}
				}
				return &IntakeError{Code: code}
			}
		}
		wait := delay/2 + time.Duration(rand.Int64N(int64(delay/2)+1))
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
		delay = min(MaxBackoff, delay*2)
	}
}

func (c *Client) attempt(ctx context.Context, req *intakeRequest) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, req.timeout)
	defer cancel()
	var body io.Reader = http.NoBody
	if len(req.body) > 0 {
		stall := newStallReader(req.body, c.stall, cancel)
		defer stall.stop()
		body = stall
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.method, c.base+req.suffix, body)
	if err != nil {
		return 0, "", err
	}
	httpReq.ContentLength = int64(len(req.body))
	httpReq.Header.Set("DD-API-KEY", c.apiKey)
	httpReq.Header.Set("Content-Type", req.contentType)
	if c.trace != nil {
		httpReq.Header.Set("x-datadog-trace-id", c.trace.TraceID)
		httpReq.Header.Set("x-datadog-parent-id", c.trace.ParentID)
		httpReq.Header.Set("x-datadog-sampling-priority", strconv.Itoa(c.trace.SamplingPriority))
	}
	for k, v := range req.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, "", nil
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return 0, "", err
	}
	code := "invalid_response"
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if len(payload) <= maxResponseBytes && json.Unmarshal(payload, &parsed) == nil && intakeErrorCodes[parsed.Error.Code] {
		code = parsed.Error.Code
	}
	return resp.StatusCode, code, nil
}

// stallReader is a request body that ends its attempt when sending makes no progress
// for the stall time. Reaching the end of the body stops the watch: waiting for the
// answer is bounded by the attempt time instead.
type stallReader struct {
	r     *bytes.Reader
	timer *time.Timer
	stall time.Duration
	once  sync.Once
	// mu orders a Reset after progress with the final stop.
	mu      sync.Mutex
	stopped bool
}

func newStallReader(body []byte, stall time.Duration, cancel context.CancelFunc) *stallReader {
	s := &stallReader{r: bytes.NewReader(body), stall: stall}
	s.timer = time.AfterFunc(stall, cancel)
	return s
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	switch {
	case err != nil || s.r.Len() == 0:
		s.stop()
	case n > 0:
		s.mu.Lock()
		if !s.stopped {
			s.timer.Reset(s.stall)
		}
		s.mu.Unlock()
	}
	return n, err
}

func (s *stallReader) Close() error { return nil }

func (s *stallReader) stop() {
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.stopped = true
		s.timer.Stop()
	})
}
