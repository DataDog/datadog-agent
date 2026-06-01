// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

func postEnvelope(ctx context.Context, client *http.Client, intakeURL, apiKey string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, intakeURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("DD-API-KEY", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	// Drain the body on the success path so the TCP connection is returned to
	// the pool. On non-2xx paths the ReadAll below reads up to 512 bytes first;
	// this defer then drains whatever remains.
	defer func() { _, _ = io.Copy(io.Discard, resp.Body) }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// %q safely escapes any non-UTF-8 bytes a proxy or error page might return.
		preview, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return fmt.Errorf("http %d (body unreadable: %w)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("http %d: %q", resp.StatusCode, preview)
	}
	return nil
}
