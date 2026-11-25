// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The upload session contract is the its-agent-intake JSON-cell format: a
// descriptor, ordered pages, then finalize or abort, all addressed by the session's
// upload ID and authenticated with the Agent's own API key alone.

const (
	intakePageTimeout    = 6 * time.Minute
	intakeRequestTimeout = 30 * time.Second
	intakeAbortTimeout   = 10 * time.Second
	intakeMaxResponse    = 64 * 1024
	intakeMaxAttempts    = 5
	intakeInitialBackoff = 100 * time.Millisecond
	intakeMaxBackoff     = 5 * time.Second
)

// intakeError carries only a stable code; response messages are never propagated.
type intakeError struct{ code string }

func (e *intakeError) Error() string { return "intake request failed: " + e.code }

var intakeErrorCodes = map[string]bool{
	"invalid_argument": true, "unauthenticated": true, "not_found": true, "already_exists": true,
	"failed_precondition": true, "gone": true, "payload_too_large": true, "result_file_too_large": true,
	"scan_failed": true, "unsupported_encoding": true, "storage_access_denied": true,
	"unavailable": true, "internal": true,
}

// intakeClient talks to one upload session. It has no redirect support, no
// configurable destination and no credential other than the API key header.
type intakeClient struct {
	http     *http.Client
	base     string
	uploadID string
	apiKey   string
	trace    *remoteTrace
	// sleep is replaced in tests.
	sleep func(context.Context, time.Duration) error
}

func newIntakeClient(client *http.Client, base, uploadID, apiKey string, trace *remoteTrace) *intakeClient {
	return &intakeClient{http: client, base: strings.TrimRight(base, "/"), uploadID: uploadID, apiKey: apiKey, trace: trace, sleep: sleepContext}
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

func (c *intakeClient) registerDescriptor(ctx context.Context, body []byte) error {
	return c.do(ctx, http.MethodPost, "/descriptor", body, "application/json", nil, intakeRequestTimeout)
}

func (c *intakeClient) putPage(ctx context.Context, index int, body []byte, rows, offset int64) error {
	headers := map[string]string{
		"X-DD-Source-Page-Rows":  strconv.FormatInt(rows, 10),
		"X-DD-Record-Offset":     strconv.FormatInt(offset, 10),
		"X-DD-Source-Page-Bytes": strconv.Itoa(len(body)),
	}
	return c.do(ctx, http.MethodPut, "/pages/"+strconv.Itoa(index), body, "text/csv", headers, intakePageTimeout)
}

func (c *intakeClient) finalize(ctx context.Context, pages int) error {
	body, _ := json.Marshal(map[string]int{"expected_page_count": pages})
	return c.do(ctx, http.MethodPost, "/finalize", body, "application/json", nil, intakeRequestTimeout)
}

// abort has its own budget and does not depend on the run context, which is already
// cancelled after a timeout or unscheduling.
func (c *intakeClient) abort() error {
	ctx, cancel := context.WithTimeout(context.Background(), intakeAbortTimeout)
	defer cancel()
	return c.do(ctx, http.MethodPost, "/abort", nil, "application/json", nil, intakeAbortTimeout)
}

// do sends one request, retrying capacity and transient failures with bounded
// exponential backoff and jitter. Every attempt sends the same bytes.
func (c *intakeClient) do(ctx context.Context, method, suffix string, body []byte, contentType string, headers map[string]string, timeout time.Duration) error {
	target := c.base + "/uploads/" + url.PathEscape(c.uploadID) + suffix
	failures := 0
	delay := intakeInitialBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, code, err := c.attempt(ctx, method, target, body, contentType, headers, timeout)
		switch {
		case err == nil && status == http.StatusNoContent:
			return nil
		case err == nil && status == http.StatusServiceUnavailable && code == "unavailable":
			// Capacity: retry until the context ends, without counting a failure.
		case err == nil && (status != http.StatusInternalServerError || code != "internal"):
			return &intakeError{code: code}
		default:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			if failures >= intakeMaxAttempts {
				if err != nil {
					return &intakeError{code: "transport_error"}
				}
				return &intakeError{code: code}
			}
		}
		wait := delay/2 + time.Duration(rand.Int64N(int64(delay/2)+1))
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
		delay = min(intakeMaxBackoff, delay*2)
	}
}

func (c *intakeClient) attempt(ctx context.Context, method, target string, body []byte, contentType string, headers map[string]string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", contentType)
	if c.trace != nil {
		req.Header.Set("x-datadog-trace-id", c.trace.TraceID)
		req.Header.Set("x-datadog-parent-id", c.trace.ParentID)
		req.Header.Set("x-datadog-sampling-priority", strconv.Itoa(c.trace.SamplingPriority))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, "", nil
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, intakeMaxResponse+1))
	if err != nil {
		return 0, "", err
	}
	code := "invalid_response"
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if len(payload) <= intakeMaxResponse && json.Unmarshal(payload, &parsed) == nil && intakeErrorCodes[parsed.Error.Code] {
		code = parsed.Error.Code
	}
	return resp.StatusCode, code, nil
}

func isIntakeCode(err error, code string) bool {
	var ie *intakeError
	return errors.As(err, &ie) && ie.code == code
}
