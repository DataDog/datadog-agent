// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
)

// stage is how far into the provision -> install -> test lifecycle a run
// should go.
type stage int

const (
	stageProvision stage = iota
	stageInstall
	stageTest
)

func (s stage) String() string {
	switch s {
	case stageProvision:
		return "provision"
	case stageInstall:
		return "install"
	case stageTest:
		return "test"
	default:
		return "unknown"
	}
}

func parseStage(s string) (stage, error) {
	switch s {
	case "provision":
		return stageProvision, nil
	case "install":
		return stageInstall, nil
	case "test":
		return stageTest, nil
	default:
		return 0, fmt.Errorf("unknown --stage %q, want one of: provision, install, test", s)
	}
}

// runLifecycle drives the def through as many of provision/install/test as
// target requires, skipping stages the state file already reflects. If
// target is nil, it prompts interactively for how far to go — the menu
// adapts to which stages are already done.
func runLifecycle(ctx context.Context, def TestDefinition, statePath string, target *stage) error {
	st, err := readStateFile(statePath)
	if err != nil {
		return fmt.Errorf("reading state file %s: %w", statePath, err)
	}

	if target == nil {
		chosen, err := promptForStage(st, def)
		if err != nil {
			return err
		}
		target = &chosen
	}

	provisioned, installed := stagesCompleted(st)

	if !provisioned {
		if err := doProvision(ctx, def, statePath); err != nil {
			return err
		}
	} else {
		fmt.Println("infra already provisioned, skipping")
	}
	if *target == stageProvision {
		return nil
	}

	upToDate := false
	if installed {
		status, err := installers.Status(def.Agent.Installer, st, installParamsFor(def))
		if err != nil {
			return err
		}
		upToDate = status.UpToDate
	}
	if !installed || !upToDate {
		if err := doInstall(ctx, def, statePath); err != nil {
			return err
		}
	} else {
		fmt.Println("agent already installed with matching config, skipping")
	}
	if *target == stageInstall {
		return nil
	}

	return doTest(ctx, def, statePath)
}

// promptForStage prints a menu adapted to st's completed stages and reads
// the user's choice from stdin. Callers must have already verified stdin is
// a terminal (see main.go) — this only handles the read/parse loop.
func promptForStage(st envState, def TestDefinition) (stage, error) {
	provisioned, installed := stagesCompleted(st)
	upToDate := false
	if installed {
		status, err := installers.Status(def.Agent.Installer, st, installParamsFor(def))
		if err != nil {
			return 0, err
		}
		upToDate = status.UpToDate
	}

	type option struct {
		s     stage
		label string
	}
	var options []option
	switch {
	case !provisioned:
		options = []option{
			{stageProvision, "provision infra only"},
			{stageInstall, "provision infra + install agent"},
			{stageTest, "provision infra + install agent + run test"},
		}
	case !upToDate:
		options = []option{
			{stageInstall, "install/update agent"},
			{stageTest, "install/update agent + run test"},
		}
	default:
		options = []option{
			{stageTest, "run test"},
		}
	}

	fmt.Println("How far should this run go?")
	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt.label)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return 0, fmt.Errorf("reading stage selection: %w", err)
			}
			return 0, errors.New("no input read for stage selection")
		}
		choice := strings.TrimSpace(scanner.Text())
		for i, opt := range options {
			if choice == fmt.Sprintf("%d", i+1) {
				return opt.s, nil
			}
		}
		fmt.Printf("please enter a number between 1 and %d\n", len(options))
	}
}

func doProvision(ctx context.Context, def TestDefinition, statePath string) error {
	cfg, err := def.provisionConfig()
	if err != nil {
		return err
	}
	p, err := resolveProvisioner(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("provisioning %q (provisioner=%s)...\n", def.Name, def.Provisioner.Type)
	resources, err := p.Provision(ctx, def.Name, os.Stdout)
	if err != nil {
		return err
	}

	st := make(envState, len(resources))
	for k, v := range resources {
		st[k] = v
	}
	if err := writeStateFileAtomic(statePath, st); err != nil {
		return err
	}
	fmt.Printf("infra provisioned, state written to %s\n", statePath)
	return nil
}

func doInstall(ctx context.Context, def TestDefinition, statePath string) error {
	apiKey, appKey, err := installers.ResolveAPIKeys()
	if err != nil {
		return err
	}
	params := installParamsFor(def)
	params.APIKey = apiKey
	params.AppKey = appKey

	fmt.Printf("installing agent %s / cluster-agent %s...\n", def.Agent.AgentVersion, def.Agent.ClusterAgentVersion)
	if err := installers.UpdateAgent(ctx, statePath, def.Agent.Installer, params); err != nil {
		return err
	}
	fmt.Printf("agent installed, %s updated\n", statePath)
	return nil
}

// installParamsFor builds the installers.InstallParams describing what
// def's YAML asks for — everything installers.Status needs to compare
// against a recorded "agent" entry. doInstall builds its own copy and
// layers API keys on top, since Status never needs those.
func installParamsFor(def TestDefinition) installers.InstallParams {
	return installers.InstallParams{
		AgentVersion:        def.Agent.AgentVersion,
		ClusterAgentVersion: def.Agent.ClusterAgentVersion,
		Namespace:           def.Agent.Namespace,
	}
}

func doTest(ctx context.Context, def TestDefinition, statePath string) error {
	// dda inv resolves modules.yml (and new-e2e-tests.run itself cd's into
	// test/new-e2e before running go test) relative to the process's working
	// directory, so both statePath and the dda invocation itself need to be
	// anchored regardless of where e2ectl was invoked from.
	absStatePath, err := filepath.Abs(statePath)
	if err != nil {
		return fmt.Errorf("resolving absolute path for %s: %w", statePath, err)
	}
	root, err := repoRoot(ctx)
	if err != nil {
		return err
	}

	args := []string{"inv", "new-e2e-tests.run", "--targets=" + def.Test.Package}
	if def.Test.Run != "" {
		args = append(args, "--run="+def.Test.Run)
	}

	fmt.Printf("running: dda %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "dda", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "E2E_ENV_FILE="+absStatePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// repoRoot finds the repository root so dda inv (which resolves modules.yml
// relative to its working directory) runs correctly no matter which
// directory e2ectl itself was invoked from.
func repoRoot(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("resolving repository root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
