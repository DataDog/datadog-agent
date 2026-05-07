// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package main implements cws-stream-events, a small standalone client for the
// system-probe CWS event stream. It connects to the
// SecurityModuleEvent.GetEventStream gRPC service over a Unix socket and
// pretty-prints every SecurityEventMessage to stdout.
//
// It exists as a debugging companion to system-probe + a SECL policy that the
// operator wants to inspect by hand. It does NOT replace security-agent; in
// particular, it does not handle reconnect retry counters, telemetry,
// rate-limit reporting, or any of the heartbeat / policy-management plumbing
// in pkg/security/agent.
//
// Usage:
//
//	sudo cws-stream-events [--socket /tmp/cws-explore/runtime-security.sock]
//
// The protobuf surface (SecurityEventMessage, SecurityModuleEventClient) lives
// in pkg/security/proto/api/ and is reused from the main module.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

func main() {
	socketPath := flag.String("socket", "/tmp/cws-explore/runtime-security.sock",
		"path to the system-probe runtime_security_config.socket")
	prettyJSON := flag.Bool("pretty", true, "pretty-print the event JSON payload")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, *socketPath, *prettyJSON); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "cws-stream-events: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, socketPath string, pretty bool) error {
	// gRPC over unix socket. The system-probe side uses the vtproto codec
	// (api.VTProtoCodecName) which auto-registers via init() in vt_grpc.go,
	// so we just need to ask for it here.
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(api.VTProtoCodecName)),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", socketPath, err)
	}
	defer conn.Close()

	client := api.NewSecurityModuleEventClient(conn)

	fmt.Fprintf(os.Stderr, "cws-stream-events: connected to %s, awaiting events (ctrl-c to stop)\n", socketPath)

	// Outer reconnect loop: if the stream drops (system-probe restart) we
	// retry every 2s, matching the cadence of the canonical security-agent
	// client at pkg/security/agent/agent.go:200.
	for {
		if err := streamOnce(ctx, client, pretty); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			fmt.Fprintf(os.Stderr, "cws-stream-events: stream error: %v (reconnecting in 2s)\n", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func streamOnce(ctx context.Context, client api.SecurityModuleEventClient, pretty bool) error {
	stream, err := client.GetEventStream(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("GetEventStream: %w", err)
	}

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg == nil {
			continue
		}
		printEvent(msg, pretty)
	}
}

func printEvent(msg *api.SecurityEventMessage, pretty bool) {
	// Header summary so it's easy to scan quickly.
	ts := ""
	if msg.Timestamp != nil {
		ts = msg.Timestamp.AsTime().Format(time.RFC3339Nano)
	}
	fmt.Printf("=== rule=%s service=%s tags=%v time=%s\n",
		msg.RuleID, msg.Service, msg.Tags, ts)

	// msg.Data is a JSON document produced by pkg/security/probe/serializers.go.
	// Try to pretty-print it; fall back to the raw bytes if it doesn't parse.
	if !pretty {
		_, _ = os.Stdout.Write(msg.Data)
		_, _ = os.Stdout.Write([]byte{'\n'})
		return
	}

	var raw any
	if err := json.Unmarshal(msg.Data, &raw); err != nil {
		fmt.Printf("  (data not valid JSON: %v)\n  %s\n", err, string(msg.Data))
		return
	}
	out, err := json.MarshalIndent(raw, "  ", "  ")
	if err != nil {
		fmt.Printf("  (failed to marshal: %v)\n", err)
		return
	}
	fmt.Printf("  %s\n", out)
}
