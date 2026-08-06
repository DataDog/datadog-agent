// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// agent-rollout-gate is a small Linux wrapper used by the experimental
// per-node Agent surge rollout. It deliberately does not initialize any Agent
// package before the previous generation releases the component lock.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	lockPathEnv     = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_LOCK_PATH"
	preparedPathEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH"
	podUIDEnv       = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
)

type options struct {
	component    string
	lockPath     string
	preparedPath string
	podUID       string
	waitFile     string
	command      []string
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "agent-rollout-gate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	return waitAndExec(opts, stderr)
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("agent-rollout-gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.component, "component", "", "Agent container component name")
	flags.StringVar(&opts.waitFile, "wait-file", "", "file that must exist and be non-empty after lock acquisition")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}

	opts.lockPath = os.Getenv(lockPathEnv)
	opts.preparedPath = os.Getenv(preparedPathEnv)
	opts.podUID = os.Getenv(podUIDEnv)
	opts.command = flags.Args()

	if opts.component == "" {
		return options{}, errors.New("--component is required")
	}
	if len(opts.command) == 0 {
		return options{}, errors.New("a command is required after --")
	}
	if !filepath.IsAbs(opts.lockPath) || filepath.Base(opts.lockPath) != opts.component+".lock" {
		return options{}, fmt.Errorf("%s must be an absolute path ending in %q", lockPathEnv, opts.component+".lock")
	}
	if !filepath.IsAbs(opts.preparedPath) || filepath.Base(opts.preparedPath) != opts.component+".prepared" {
		return options{}, fmt.Errorf("%s must be an absolute path ending in %q", preparedPathEnv, opts.component+".prepared")
	}
	if filepath.Dir(opts.lockPath) != filepath.Dir(opts.preparedPath) {
		return options{}, errors.New("lock and Prepared marker must share a directory")
	}
	if opts.podUID == "" {
		return options{}, fmt.Errorf("%s is required", podUIDEnv)
	}
	if opts.waitFile != "" && !filepath.IsAbs(opts.waitFile) {
		return options{}, errors.New("--wait-file must be an absolute path")
	}
	return opts, nil
}
