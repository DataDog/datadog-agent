// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main is the feature-branch-only EUDM simulator entrypoint.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/command"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture/native"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/engine"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runtime := command.Runtime{Capture: func(ctx context.Context, request command.CaptureRequest) error {
		return native.Run(ctx, request.Directory)
	}, Replay: func(ctx context.Context, request command.ReplayRequest) error {
		return engine.Execute(ctx, engine.Request{Scenario: request.Scenario, Plan: request.Plan, Bundles: request.Bundles}, engine.ExecutionOptions{Workers: request.Workers, QueueCapacity: request.QueueCapacity, DeliveryGrace: request.DeliveryGrace, ReportPath: request.ReportPath, APIKey: os.Getenv("DD_API_KEY"), Destinations: request.Destinations})
	}}
	if err := command.MakeCommand(runtime).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
