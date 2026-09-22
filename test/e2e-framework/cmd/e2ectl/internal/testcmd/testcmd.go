// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testcmd implements `e2ectl test`: a thin dispatcher that runs
// existing new-e2e suites against a live, e2ectl-owned environment.
//
// It is deliberately NOT a test runner: the child process is plain
// `go test -tags test`, the suite code and output are exactly what the
// developer would see without e2ectl. The command resolves the
// environment, verifies it is ready with the agent installed, exports the
// attach variables (E2ECTL_ENV, E2ECTL_HOME) and pre-selects the right
// entry point per environment base so provisioning-based entries in the
// same suite never fire. It never provisions, never destroys, never
// rebuilds — those are start/install/update, by design.
package testcmd

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
)

// defaultRunPattern returns the default `go test -run` pattern for an
// environment base: attachable entry points follow the <Test>On<Affix>
// convention (OnLocal, OnLocalKind, OnHost). The pattern keeps
// provisioning-based entry points in the same suite from firing when the
// suite target includes them. An explicit -run overrides it; an explicit
// empty -run ("-run ”") runs every test in the suite.
func defaultRunPattern(base string) (string, bool) {
	switch base {
	case "local", "kind":
		// OnLocalKind contains OnLocal, so one pattern covers both affixes.
		return "OnLocal", true
	case "ec2-host":
		return "OnHost", true
	case "docker-host":
		return "OnDockerHost", true
	case "eks":
		return "OnEKS", true
	default:
		return "", false
	}
}

// Run implements `e2ectl test --env <name> --suite <go packages> [--run pattern] [-- extra go test args]`.
func Run(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	name := fs.String("env", "", "environment name (required; e2ectl start + e2ectl install first)")
	suite := fs.String("suite", "", "go test target(s), repo-root-relative, e.g. ./test/new-e2e/tests/containers/ (required)")
	run := fs.String("run", "\x00", "go test -run pattern (default: the attach entry-point pattern for the environment's base; set '' to run every test)")
	verbose := fs.Bool("v", true, "pass -v to go test")
	timeout := fs.String("timeout", "90m", "go test -timeout for the whole suite (suites with warmup waits need more than the 10m go-test default)")
	cached := fs.Bool("cached", false, "allow go test caching; by default -count=1 forces a real run (the attached environment is mutable state go test cannot see, so a cache hit would be a false PASS)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--env is required")
	}
	if *suite == "" {
		return fmt.Errorf("--suite is required (e.g. --suite ./test/new-e2e/tests/containers/)")
	}
	extra := fs.Args()

	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Get(*name)
	if err != nil {
		return fmt.Errorf("environment %q not found — start one first:\n  e2ectl init --base kind --output %s.yaml\n  e2ectl start --config %s.yaml --name %s", *name, *name, *name, *name)
	}
	switch entry.Meta.Status {
	case "ready":
	default:
		return fmt.Errorf("environment %q is %q — wait for it or check e2ectl list", *name, entry.Meta.Status)
	}
	if !entry.Meta.AgentInstalled {
		return fmt.Errorf("agent not installed on %q — run:\n  e2ectl install -env %s", *name, *name)
	}

	state, err := installer.RoutingStatus(entry)
	if err != nil {
		return err
	}
	if state.Phase != "" && state.Phase != "applied" {
		return fmt.Errorf("Agent routing is %s; repair it before test attachment", state.Phase)
	}
	fmt.Fprintf(os.Stderr, "Agent routing: %s; delivery: %s (retained fixture data may belong to older/other senders)\n", state.Phase, state.Delivery)
	goArgs := []string{"test", "-tags", "test", "-timeout", *timeout}
	// The suite attaches to a live, mutable environment (receiver changes,
	// reinstalls, new payloads) that is invisible to go test's cache key: an
	// unchanged command line must still re-run against it. -count=1 disables
	// caching; `-- -count=N` in extra args still overrides it (go test takes
	// the last -count).
	if !*cached {
		goArgs = append(goArgs, "-count", "1")
	}
	if *verbose {
		goArgs = append(goArgs, "-v")
	}
	if *run == "\x00" { // not set: pick the base default
		pattern, ok := defaultRunPattern(entry.Meta.Base)
		if !ok {
			return fmt.Errorf("no default test entry-point pattern for base %q — pass --run explicitly", entry.Meta.Base)
		}
		goArgs = append(goArgs, "-run", pattern)
	} else if *run != "" { // explicit empty string = run everything
		goArgs = append(goArgs, "-run", *run)
	}
	goArgs = append(goArgs, strings.Fields(*suite)...)
	goArgs = append(goArgs, extra...)

	cmd := exec.Command("go", goArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"E2ECTL_ENV="+*name,
		"E2ECTL_HOME="+store.Root(),
	)
	if _, err := os.Stat("go.work"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: no go.work in the working directory — e2ectl test must run from the repository root\n")
	}
	fmt.Printf("running: go %s (E2ECTL_ENV=%s)\n", strings.Join(goArgs, " "), *name)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Propagate the go test exit code, not e2ectl's error exit:
			// the output above is the real go test output.
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("running go test: %w", err)
	}
	return nil
}
