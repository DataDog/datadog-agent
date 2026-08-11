// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command e2ectl provisions an environment, installs a chosen Agent
// version into it, and runs a go test against it — all without Pulumi,
// driven by a single YAML test definition (see config.go). Run
// interactively it prompts for how far to go (infra only / infra+agent /
// infra+agent+test), adapting the menu to whichever stages a previous run
// already completed; run with --stage or --yes it skips the prompt for
// scripted/CI use. Each stage's result lives in a JSON state file (see
// state.go) with the same shape `go test` already consumes via
// E2E_ENV_FILE (see test/new-e2e/examples/kind_nopulumi_test.go) — e2ectl
// only changes how that file gets produced, not what reads it.
//
// Usage:
//
//	e2ectl run --config=<test.yaml> [--state=<path>] [--stage=provision|install|test] [--yes]
//	e2ectl destroy --config=<test.yaml> [--state=<path>]
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "run":
		err = runRun(os.Args[2:])
	case "destroy":
		err = runDestroy(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: e2ectl <run|destroy> [flags]")
}

func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the test definition YAML (required)")
	statePath := fs.String("state", "", "path to the state file (default: <repo-root>/test/e2e-framework/.e2ectl-state/<name>.state.json)")
	stageFlag := fs.String("stage", "", "provision|install|test — skip the interactive prompt and run up to this stage")
	yes := fs.Bool("yes", false, "non-interactive: skip the prompt (implies --stage=test unless --stage is also given)")
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
	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		return fmt.Errorf("resolving absolute path for %s: %w", *configPath, err)
	}
	if *statePath == "" {
		root, err := repoRoot(ctx)
		if err != nil {
			return err
		}
		*statePath = defaultStatePath(root, def.Name)
	}

	var target *stage
	if *stageFlag != "" {
		s, err := parseStage(*stageFlag)
		if err != nil {
			return err
		}
		target = &s
	} else if *yes {
		s := stageTest
		target = &s
	} else if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("stdin is not a terminal — pass --stage or --yes for non-interactive use")
	}

	if target != nil {
		return runLifecycle(ctx, def, absConfigPath, *statePath, *target)
	}

	scanner := bufio.NewScanner(os.Stdin)
	_, err = runEnvLoop(ctx, def, absConfigPath, *statePath, false, scanner)
	return err
}
