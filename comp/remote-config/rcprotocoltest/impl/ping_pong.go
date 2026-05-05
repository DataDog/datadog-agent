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
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// PingPonger is a bidirectional echo interface satisfied by each transport
// protocol (WebSocket, gRPC, TCP).
type PingPonger interface {
	Recv(ctx context.Context) ([]byte, error)
	Send(ctx context.Context, data []byte) error
	GracefulClose()
}

// RunTransportTests performs a pre-flight HTTP check and then runs the echo
// test over each supported transport protocol (WebSocket, gRPC, TCP).
//
// Each transport test is independent — a failure in one does not prevent the
// others from running.
func RunTransportTests(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) {
	log.Debug("starting remote config transport echo tests")

	if err := preflightCheck(ctx, httpClient, runCount); err != nil {
		log.Debugf("transport echo pre-flight check failed: %s", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); runWebSocketTest(ctx, httpClient, runCount) }()
	go func() { defer wg.Done(); runWebSocketTestWithALPN(ctx, httpClient, runCount) }()
	go func() { defer wg.Done(); runGrpcTest(ctx, httpClient, runCount) }()
	go func() { defer wg.Done(); runTCPTest(ctx, httpClient) }()
	wg.Wait()

	log.Debug("remote config transport echo tests complete")
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

	n, err := runEchoLoop(ctx, httpClient, runCount, ALPNDefault)
	if err != nil {
		log.Debugf("websocket echo test failed: %s (%d data frames exchanged)", err, n)
		return
	}
	log.Debugf("websocket echo test complete (%d data frames exchanged)", n)
}

func runWebSocketTestWithALPN(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("unexpected websocket echo with ALPN connectivity test failure: %s", err)
		}
	}()

	// ALPN requires TLS, check if TLS is enabled before running test.
	baseURL, err := httpClient.BaseURL()
	if err != nil {
		log.Debugf("websocket echo test with ALPN failed to get base URL: %s", err)
		return
	}
	if strings.ToLower(baseURL.Scheme) == "http" {
		log.Debug("websocket echo test with ALPN skipped: TLS is disabled")
		return
	}

	n, err := runEchoLoop(ctx, httpClient, runCount, ALPNDDRC)
	if err != nil {
		log.Debugf("websocket echo test with ALPN failed: %s (%d data frames exchanged)", err, n)
		return
	}
	log.Debugf("websocket echo test with ALPN complete (%d data frames exchanged)", n)
}

func runGrpcTest(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("unexpected grpc echo connectivity test failure: %s", err)
		}
	}()

	pp, err := NewGrpcPingPonger(ctx, httpClient, runCount)
	if err != nil {
		log.Debugf("grpc echo test init failed: %s", err)
		return
	}
	defer pp.GracefulClose()

	n, err := runPingPong(ctx, pp)
	if err != nil {
		log.Debugf("grpc echo test failed: %s (%d frames exchanged)", err, n)
		return
	}
	log.Debugf("grpc echo test complete (%d frames exchanged)", n)
}

func runTCPTest(ctx context.Context, httpClient *api.HTTPClient) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("unexpected tcp echo connectivity test failure: %s", err)
		}
	}()

	pp, err := NewTCPPingPonger(ctx, httpClient)
	if err != nil {
		log.Debugf("tcp echo test init failed: %s", err)
		return
	}
	defer pp.GracefulClose()

	n, err := runPingPong(ctx, pp)
	if err != nil {
		log.Debugf("tcp echo test failed: %s (%d frames exchanged)", err, n)
		return
	}
	log.Debugf("tcp echo test complete (%d frames exchanged)", n)
}

func runPingPong(ctx context.Context, client PingPonger) (uint, error) {
	// Perform the frame echo test routine.
	n := uint(0)
	for {
		select {
		case <-ctx.Done():
			return n, context.Cause(ctx)
		default:
		}

		buf, err := client.Recv(ctx)
		if err != nil {
			return n, err
		}

		if err := client.Send(ctx, buf); err != nil {
			return n, err
		}

		n++
	}
}
