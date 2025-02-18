// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements the setup logic called by the install scripts for multiple flavors.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

func main() {
	env := env.FromEnv()
	ctx := context.Background()
	t := telemetry.NewTelemetry(env.HTTPClient(), env.APIKey, env.Site, fmt.Sprintf("datadog-installer-setup-%s", flavor))
	span, ctx := telemetry.StartSpanFromEnv(ctx, fmt.Sprintf("setup-%s", flavor))
	err := run(ctx)
	span.Finish(err)
	t.Stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}
}
