// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcprotocoltestimpl

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// RunEchoTest performs a pre-flight HTTP check and then runs the WebSocket
// echo test.
func RunEchoTest(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) {
	log.Debug("starting remote config websocket echo test")

	if err := preflightCheck(ctx, httpClient, runCount); err != nil {
		log.Debugf("websocket echo pre-flight check failed: %s", err)
		return
	}

	runWebSocketTest(ctx, httpClient, runCount)
	log.Debug("remote config websocket echo test complete")
}

// preflightCheck performs an HTTP GET to the echo-test endpoint. If the
// backend does not return 200 OK, the echo tests should not proceed.
func preflightCheck(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) error {
	baseURL, err := httpClient.BaseURL()
	if err != nil {
		return err
	}
	baseURL.Path = path.Join(baseURL.Path, "/api/v0.2/ping-pong")

	transport, err := httpClient.Transport()
	if err != nil {
		return err
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create pre-flight request: %w", err)
	}
	for k, v := range httpClient.Headers() {
		req.Header[k] = v
	}
	req.Header.Set("X-Echo-Run-Count", strconv.FormatUint(runCount, 10))
	req.Header.Set("X-Agent-UUID", uuid.GetUUID())

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pre-flight request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pre-flight returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func runWebSocketTest(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("unexpected websocket echo connectivity test failure: %s", err)
		}
	}()

	n, err := runEchoLoop(ctx, httpClient, runCount)
	if err != nil {
		log.Debugf("websocket echo test failed: %s (%d data frames exchanged)", err, n)
		return
	}
	log.Debugf("websocket echo test complete (%d data frames exchanged)", n)
}
