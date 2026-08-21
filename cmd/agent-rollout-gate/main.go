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
	"strings"
	"time"
)

const (
	defaultStateDir       = "/var/run/datadog-agent-rollout"
	legacyLockPathEnv     = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_LOCK_PATH"
	legacyPreparedPathEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH"
	legacyActivePathEnv   = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_ACTIVE_PATH"
	podUIDEnv             = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
	podIPEnv              = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_IP"
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
	component              string
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
	var stateDir string
	flags := flag.NewFlagSet("agent-rollout-gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.component, "component", "", "Agent container component name")
	flags.StringVar(&stateDir, "state-dir", "", "directory containing host-local rollout state (defaults to "+defaultStateDir+")")
	flags.StringVar(&opts.waitFile, "wait-file", "", "file that must exist and be non-empty after lock acquisition")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}

	opts.podUID = os.Getenv(podUIDEnv)
	opts.command = flags.Args()

	lockPath, preparedPath, activePath, err := resolveStatePaths(opts.component, stateDir)
	if err != nil {
		return options{}, err
	}
	opts.lockPath = lockPath
	opts.preparedPath = preparedPath
	opts.activePath = activePath
	if len(opts.command) == 0 {
		return options{}, errors.New("a command is required after --")
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
	var stateDir string
	flags := flag.NewFlagSet("agent-rollout-gate probe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.component, "component", "", "Agent container component name")
	flags.StringVar(&stateDir, "state-dir", "", "directory containing host-local rollout state (defaults to "+defaultStateDir+")")
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

	opts.podUID = os.Getenv(podUIDEnv)
	opts.address = os.Getenv(podIPEnv)
	_, preparedPath, activePath, err := resolveStatePaths(opts.component, stateDir)
	if err != nil {
		return probeOptions{}, err
	}
	opts.preparedPath = preparedPath
	opts.activePath = activePath
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

func resolveStatePaths(component, stateDir string) (string, string, string, error) {
	if component == "" {
		return "", "", "", errors.New("--component is required")
	}
	if filepath.Base(component) != component || component == "." || component == ".." {
		return "", "", "", errors.New("--component must be a single path element")
	}
	if stateDir != "" {
		if !filepath.IsAbs(stateDir) {
			return "", "", "", errors.New("--state-dir must be absolute")
		}
		return derivedStatePaths(component, stateDir)
	}

	legacy := []string{os.Getenv(legacyLockPathEnv), os.Getenv(legacyPreparedPathEnv), os.Getenv(legacyActivePathEnv)}
	if legacy[0] == "" && legacy[1] == "" && legacy[2] == "" {
		return derivedStatePaths(component, defaultStateDir)
	}
	if legacy[0] == "" || legacy[1] == "" || legacy[2] == "" {
		return "", "", "", errors.New("legacy lock, Prepared, and Active paths must be configured together")
	}
	wantNames := []string{component + ".lock", component + ".prepared", component + ".active"}
	for i, path := range legacy {
		if !filepath.IsAbs(path) || filepath.Base(path) != wantNames[i] {
			return "", "", "", fmt.Errorf("legacy rollout path must be absolute and end in %q", wantNames[i])
		}
		if filepath.Dir(path) != filepath.Dir(legacy[0]) {
			return "", "", "", errors.New("legacy lock, Prepared, and Active paths must share a directory")
		}
	}
	return legacy[0], legacy[1], legacy[2], nil
}

func derivedStatePaths(component, stateDir string) (string, string, string, error) {
	return filepath.Join(stateDir, component+".lock"), filepath.Join(stateDir, component+".prepared"), filepath.Join(stateDir, component+".active"), nil
}

func agentEnvironment(environment []string) []string {
	clean := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if name == legacyLockPathEnv || name == legacyPreparedPathEnv || name == legacyActivePathEnv || name == podUIDEnv || name == podIPEnv {
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}
