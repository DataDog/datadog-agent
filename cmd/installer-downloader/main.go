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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

var (
	// flavorPaths maps the flavor to the relevent setup file.
	// This is used to link users to the relevent code.
	flavorPaths = map[string]string{
		"agent":      "pkg/fleet/installer/setup/defaultscript/defaultscript.go",
		"databricks": "pkg/fleet/installer/setup/djm/databrics.go",
		"dataproc":   "pkg/fleet/installer/setup/djm/dataproc.go",
		"emr":        "pkg/fleet/installer/setup/djm/emr.go",
	}

	// Flavor is the Flavor of the setup script. It is set at build time.
	Flavor string
	// Version is the version of the setup script. It is set at build time.
	Version string
)

func main() {
	env := env.FromEnv()
	ctx := context.Background()
	t := telemetry.NewTelemetry(env.HTTPClient(), env.APIKey, env.Site, fmt.Sprintf("datadog-installer-setup-%s", Flavor))
	span, ctx := telemetry.StartSpanFromEnv(ctx, fmt.Sprintf("setup-%s", Flavor))
	err := runFlavor(ctx, env)
	span.Finish(err)
	t.Stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}
}

func runFlavor(ctx context.Context, env *env.Env) error {
	s, err := common.NewSetup(ctx, env, Flavor, flavorPaths[Flavor], os.Stdout)
	if err != nil {
		return err
	}
	err = run(s)
	if err != nil {
		return err
	}
	return s.Run()
}
