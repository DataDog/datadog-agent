// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

const maxStatusBytes int64 = 128 << 10

// JSON string escaping can expand semantic bytes sixfold. Read batches stream
// through a bounded decoder; no unbounded response-body helper is used.
const maxBatchBytes = 6*tc.MaxBytes + maxStatusBytes

var errResponse = errors.New("invalid capture API response")

type httpClient struct {
	role, base, token string
	http              *http.Client
}

func newHTTPClient(role, base, token string, client *http.Client) *httpClient {
	owned := *client
	owned.Timeout = 10 * time.Second
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &httpClient{role: role, base: base, token: token, http: &owned}
}

func (c *httpClient) Role() string { return c.role }

func (c *httpClient) request(ctx context.Context, operation string, value any, limit int64) (*http.Response, error) {
	method := http.MethodGet
	var body io.Reader
	if value != nil {
		method = http.MethodPost
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, errResponse
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/"+operation, body)
	if err != nil {
		return nil, errResponse
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, ErrAuthentication
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			return nil, ErrIncompatible
		case http.StatusServiceUnavailable:
			return nil, ErrUnavailable
		default:
			return nil, fmt.Errorf("capture API operation failed (HTTP %d)", response.StatusCode)
		}
	}
	if response.ContentLength > limit || (response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity") {
		response.Body.Close()
		return nil, errResponse
	}
	return response, nil
}

func finishJSON(decoder *json.Decoder, bounded *io.LimitedReader) error {
	if decoder.Decode(new(any)) != io.EOF || bounded.N <= 0 {
		return errResponse
	}
	return nil
}

func (c *httpClient) control(ctx context.Context, operation string, value any) (tc.Status, error) {
	response, err := c.request(ctx, operation, value, maxStatusBytes)
	if err != nil {
		return tc.Status{}, err
	}
	defer response.Body.Close()
	bounded := &io.LimitedReader{R: response.Body, N: maxStatusBytes + 1}
	decoder := json.NewDecoder(bounded)
	decoder.DisallowUnknownFields()
	var status tc.Status
	if decoder.Decode(&status) != nil || finishJSON(decoder, bounded) != nil {
		return tc.Status{}, errResponse
	}
	return status, nil
}

func (c *httpClient) Capabilities(ctx context.Context) (tc.Status, error) {
	return c.control(ctx, "capabilities", nil)
}
func (c *httpClient) Prepare(ctx context.Context, request tc.PrepareRequest) (tc.Status, error) {
	return c.control(ctx, "prepare", request)
}
func (c *httpClient) Activate(ctx context.Context, request tc.Control) (tc.Status, error) {
	return c.control(ctx, "activate", request)
}
func (c *httpClient) Heartbeat(ctx context.Context, request tc.Control) (tc.Status, error) {
	return c.control(ctx, "heartbeat", request)
}
func (c *httpClient) RequestHostSystemInfo(ctx context.Context, request tc.Control) (tc.Status, error) {
	return c.control(ctx, "host-system-info", request)
}
func (c *httpClient) Stop(ctx context.Context, request tc.Control) (tc.Status, error) {
	return c.control(ctx, "stop", request)
}

func (c *httpClient) Records(ctx context.Context, request tc.ReadRequest) (Batch, error) {
	response, err := c.request(ctx, "records", request, maxBatchBytes)
	if err != nil {
		return Batch{}, err
	}
	defer response.Body.Close()
	batch, err := decodeBatch(response.Body)
	if err != nil && ctx.Err() != nil {
		return Batch{}, ctx.Err()
	}
	return batch, err
}

func decodeBatch(body io.Reader) (Batch, error) {
	bounded := &io.LimitedReader{R: body, N: maxBatchBytes + 1}
	decoder := json.NewDecoder(bounded)
	decoder.DisallowUnknownFields()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return Batch{}, errResponse
	}
	var batch Batch
	seen := map[string]bool{}
	var size int64
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return Batch{}, errResponse
		}
		seen[key] = true
		switch key {
		case "status":
			if decoder.Decode(&batch.Status) != nil {
				return Batch{}, errResponse
			}
		case "records":
			if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
				return Batch{}, errResponse
			}
			for decoder.More() {
				if len(batch.Records) >= tc.MaxBatchRecords {
					return Batch{}, errResponse
				}
				var record tc.Record
				if decoder.Decode(&record) != nil {
					return Batch{}, errResponse
				}
				itemSize := tc.PayloadSize(record.Payload) + tc.RecordOverhead
				size += itemSize
				if itemSize > tc.MaxItemBytes || size > tc.MaxBytes {
					return Batch{}, errResponse
				}
				batch.Records = append(batch.Records, record)
			}
			if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
				return Batch{}, errResponse
			}
		default:
			return Batch{}, errResponse
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') || !seen["status"] || !seen["records"] || finishJSON(decoder, bounded) != nil {
		return Batch{}, errResponse
	}
	return batch, nil
}
