// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"errors"
	"flag"
)

// runDestroy tears down the infra described by a TestDefinition. It remains
// a standalone, scriptable subcommand for non-interactive/CI use (no menu,
// no confirmation prompt — the caller is trusted to mean it). The
// interactive per-env loop (runEnvLoop, wizard.go) also offers teardown as
// menu action "4", gated behind confirmDestroy's type-the-name-back
// confirmation; both paths funnel through doDestroy so there's exactly one
// teardown implementation.
func runDestroy(args []string) error {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the test definition YAML (required)")
	statePath := fs.String("state", "", "path to the state file written by 'run' (default: <repo-root>/test/e2e-framework/.e2ectl-state/<name>.state.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	def, err := loadTestDefinition(*configPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if *statePath == "" {
		root, err := repoRoot(ctx)
		if err != nil {
			return err
		}
		*statePath = defaultStatePath(root, def.Name)
	}

	return doDestroy(ctx, def, *statePath)
}
