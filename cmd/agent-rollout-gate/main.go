// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// agent-rollout-gate is a small Linux wrapper used by the experimental
// per-node overlapping Agent rollout. It deliberately does not initialize any Agent
// package before the previous generation releases the component lock.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	lockPathEnv     = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_LOCK_PATH"
	preparedPathEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH"
	activePathEnv   = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_ACTIVE_PATH"
	podUIDEnv       = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
	podIPEnv        = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_IP"
)

type options struct {
	component    string
	lockPath     string
	preparedPath string
	activePath   string
	podUID       string
	waitFile     string
	command      []string
}

type probeOptions struct {
	kind                   string
	handler                string
	port                   int
	path                   string
	timeout                time.Duration
	failureThreshold       int
	terminationGracePeriod time.Duration
	address                string
	preparedPath           string
	activePath             string
	podUID                 string
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "agent-rollout-gate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "probe" {
		opts, err := parseProbeOptions(args[1:], stderr)
		if err != nil {
			return err
		}
		return runProbe(opts)
	}
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
	opts.activePath = os.Getenv(activePathEnv)
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
	if !filepath.IsAbs(opts.activePath) || filepath.Base(opts.activePath) != opts.component+".active" {
		return options{}, fmt.Errorf("%s must be an absolute path ending in %q", activePathEnv, opts.component+".active")
	}
	if filepath.Dir(opts.lockPath) != filepath.Dir(opts.preparedPath) || filepath.Dir(opts.lockPath) != filepath.Dir(opts.activePath) {
		return options{}, errors.New("lock, Prepared marker, and Active marker must share a directory")
	}
	if opts.podUID == "" {
		return options{}, fmt.Errorf("%s is required", podUIDEnv)
	}
	if opts.waitFile != "" && !filepath.IsAbs(opts.waitFile) {
		return options{}, errors.New("--wait-file must be an absolute path")
	}
	return opts, nil
}

func parseProbeOptions(args []string, stderr io.Writer) (probeOptions, error) {
	var opts probeOptions
	flags := flag.NewFlagSet("agent-rollout-gate probe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.kind, "kind", "", "probe kind: startup")
	flags.StringVar(&opts.handler, "handler", "active", "active, http, or tcp")
	flags.IntVar(&opts.port, "port", 0, "Pod HTTP or TCP port")
	flags.StringVar(&opts.path, "path", "", "HTTP request path")
	flags.DurationVar(&opts.timeout, "timeout", time.Second, "network probe timeout")
	flags.IntVar(&opts.failureThreshold, "failure-threshold", 3, "consecutive Active startup failures before terminating the Agent")
	flags.DurationVar(&opts.terminationGracePeriod, "termination-grace-period", 30*time.Second, "grace between SIGTERM and SIGKILL after the Active startup failure threshold")
	if err := flags.Parse(args); err != nil {
		return probeOptions{}, err
	}
	if flags.NArg() != 0 {
		return probeOptions{}, errors.New("probe does not accept positional arguments")
	}

	opts.preparedPath = os.Getenv(preparedPathEnv)
	opts.activePath = os.Getenv(activePathEnv)
	opts.podUID = os.Getenv(podUIDEnv)
	opts.address = os.Getenv(podIPEnv)
	if !filepath.IsAbs(opts.preparedPath) || !filepath.IsAbs(opts.activePath) || filepath.Dir(opts.preparedPath) != filepath.Dir(opts.activePath) {
		return probeOptions{}, errors.New("Prepared and Active marker paths must be absolute and share a directory")
	}
	if opts.podUID == "" {
		return probeOptions{}, fmt.Errorf("%s is required", podUIDEnv)
	}
	if opts.timeout <= 0 {
		return probeOptions{}, errors.New("--timeout must be positive")
	}
	if opts.failureThreshold <= 0 {
		return probeOptions{}, errors.New("--failure-threshold must be positive")
	}
	if opts.terminationGracePeriod <= 0 {
		return probeOptions{}, errors.New("--termination-grace-period must be positive")
	}
	if opts.kind != "startup" {
		return probeOptions{}, errors.New("--kind must be startup")
	}
	if opts.handler != "active" && opts.handler != "http" && opts.handler != "tcp" {
		return probeOptions{}, errors.New("startup probe handler must be active, http, or tcp")
	}

	if opts.handler == "http" {
		if net.ParseIP(opts.address) == nil {
			return probeOptions{}, fmt.Errorf("%s must contain a valid Pod IP", podIPEnv)
		}
		if opts.port <= 0 || opts.port > 65535 || opts.path == "" || opts.path[0] != '/' {
			return probeOptions{}, errors.New("HTTP probe requires a valid --port and absolute --path")
		}
	} else if opts.handler == "tcp" {
		if net.ParseIP(opts.address) == nil {
			return probeOptions{}, fmt.Errorf("%s must contain a valid Pod IP", podIPEnv)
		}
		if opts.port <= 0 || opts.port > 65535 || opts.path != "" {
			return probeOptions{}, errors.New("TCP probe requires a valid --port and no --path")
		}
	} else if opts.port != 0 || opts.path != "" {
		return probeOptions{}, errors.New("active probe does not accept --port or --path")
	}
	return opts, nil
}
