// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"context"
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

// runLifecycle drives def through provision -> install -> test up to
// target, skipping stages the state file already reflects. Used only for
// non-interactive runs (--stage or --yes) — interactive runs use
// runEnvLoop instead, which offers every stage on every pass rather than
// picking one target up front.
func runLifecycle(ctx context.Context, def TestDefinition, configPath, statePath string, target stage) error {
	st, err := readStateFile(statePath)
	if err != nil {
		return fmt.Errorf("reading state file %s: %w", statePath, err)
	}

	provisioned, installed := stagesCompleted(st)

	if !provisioned {
		if err := doProvision(ctx, def, configPath, statePath); err != nil {
			return err
		}
	} else {
		fmt.Println("infra already provisioned, skipping")
	}
	if target == stageProvision {
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
	if target == stageInstall {
		return nil
	}

	return doTest(ctx, def, statePath)
}

func doProvision(ctx context.Context, def TestDefinition, configPath, statePath string) error {
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

	st, err := readStateFile(statePath)
	if err != nil {
		return fmt.Errorf("reading state file %s: %w", statePath, err)
	}
	for k, v := range resources {
		st[k] = v
	}
	if err := setSourcePath(st, configPath); err != nil {
		return err
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

// loopOutcome tells a per-env loop's caller what to do next: exit the
// process entirely, or (only reachable when the loop was entered from the
// dashboard) return to it.
type loopOutcome int

const (
	loopQuit loopOutcome = iota
	loopBackToDashboard
)

// runEnvLoop is e2ectl's interactive per-environment menu: unlike
// runLifecycle (which drives straight through to one target stage and
// exits), it reprints def's live status and offers every action on every
// pass, looping after each one — including after a failure, which is
// printed but never ends the loop — until the user quits or (if
// cameFromDashboard) asks to go back. Both `e2ectl run --config=...`
// (interactively, cameFromDashboard=false) and the dashboard
// (cameFromDashboard=true, see dashboard.go) enter through here, sharing
// one *bufio.Scanner so no buffered stdin bytes are lost switching between
// the two.
func runEnvLoop(ctx context.Context, def TestDefinition, configPath, statePath string, cameFromDashboard bool, scanner *bufio.Scanner) (loopOutcome, error) {
	for {
		st, err := readStateFile(statePath)
		if err != nil {
			return loopQuit, fmt.Errorf("reading state file %s: %w", statePath, err)
		}
		provisioned, installed := stagesCompleted(st)

		var agentStatus installers.InstallStatus
		var agentStatusErr error
		if installed {
			agentStatus, agentStatusErr = installers.Status(def.Agent.Installer, st, installParamsFor(def))
		}

		fmt.Printf("\n%s  (%s)\n", def.Name, configPath)
		if provisioned {
			fmt.Println("  infra:  provisioned")
		} else {
			fmt.Println("  infra:  not provisioned")
		}
		switch {
		case !installed:
			fmt.Println("  agent:  not installed")
		case agentStatusErr != nil:
			fmt.Println("  agent:  installed (status unavailable:", agentStatusErr, ")")
		default:
			fmt.Printf("  agent:  %s\n", agentStatus.Summary)
		}
		fmt.Printf("  test:   %s%s\n", def.Test.Package, testRunSuffix(def.Test.Run))

		fmt.Println()
		fmt.Println("1) provision infra")
		fmt.Println("2) install/update agent")
		fmt.Println("3) run test")
		fmt.Println("4) destroy environment")
		if cameFromDashboard {
			fmt.Println("b) back to dashboard")
		}
		fmt.Println("q) quit")
		fmt.Print("> ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return loopQuit, fmt.Errorf("reading menu selection: %w", err)
			}
			return loopQuit, nil
		}

		switch strings.TrimSpace(scanner.Text()) {
		case "1":
			if provisioned {
				fmt.Println("infra already provisioned, skipping")
				continue
			}
			if err := doProvision(ctx, def, configPath, statePath); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
		case "2":
			if installed && agentStatusErr == nil && agentStatus.UpToDate {
				fmt.Println("agent already installed with matching config, skipping")
				continue
			}
			if err := doInstall(ctx, def, statePath); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
		case "3":
			if err := doTest(ctx, def, statePath); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
		case "4":
			confirmed, err := confirmDestroy(scanner, def.Name)
			if err != nil {
				return loopQuit, err
			}
			if !confirmed {
				fmt.Println("destroy cancelled")
				continue
			}
			if err := doDestroy(ctx, def, statePath); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			if cameFromDashboard {
				return loopBackToDashboard, nil
			}
		case "b":
			if cameFromDashboard {
				return loopBackToDashboard, nil
			}
			fmt.Println("please enter one of the listed options")
		case "q":
			return loopQuit, nil
		default:
			fmt.Println("please enter one of the listed options")
		}
	}
}

// confirmDestroy asks the user to type name back before a destroy proceeds
// — the one action in the loop that's hard to undo.
func confirmDestroy(scanner *bufio.Scanner, name string) (bool, error) {
	fmt.Printf("type the environment name (%s) to confirm destroy: ", name)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading destroy confirmation: %w", err)
		}
		return false, nil
	}
	return strings.TrimSpace(scanner.Text()) == name, nil
}

// doDestroy tears down def's infra and removes its state file. Shared by
// the `destroy` subcommand (destroy.go) and the loop's "4) destroy
// environment" action.
func doDestroy(ctx context.Context, def TestDefinition, statePath string) error {
	cfg, err := def.provisionConfig()
	if err != nil {
		return err
	}
	p, err := resolveProvisioner(cfg)
	if err != nil {
		return err
	}

	if err := p.Destroy(ctx, def.Name, os.Stdout); err != nil {
		return err
	}
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("environment %q destroyed, %s removed\n", def.Name, statePath)
	return nil
}

// testRunSuffix formats def.Test.Run for display: "" if no -run pattern is
// set, " -run <pattern>" otherwise.
func testRunSuffix(run string) string {
	if run == "" {
		return ""
	}
	return " -run " + run
}
