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

// runDestroy tears down the infra described by a TestDefinition. It's kept
// as its own explicit subcommand, separate from the run wizard's forward
// progress, since teardown is destructive and shouldn't be one keystroke
// away inside a menu.
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
