// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const httpTimeout = 10 * time.Second

// postEnvelope POSTs the prebuilt JSON body to the EvP intake URL with the
// supplied API key. The URL is passed in by the caller (not hardcoded) so
// the same binary can target staging, dev, or a local mock.
func postEnvelope(ctx context.Context, url, apiKey string, body []byte) error {
	if url == "" {
		return fmt.Errorf("empty intake URL")
	}
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("DD-API-KEY", apiKey)
	}

	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("intake returned status %d: %s", resp.StatusCode, excerpt)
	}
	return nil
}
