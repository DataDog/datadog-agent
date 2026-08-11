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
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
)

// discoveredEnv is one environment the dashboard found by scanning a
// directory for state files and following each one's recorded "_source"
// back to its TestDefinition YAML.
type discoveredEnv struct {
	Def        TestDefinition
	ConfigPath string
	StatePath  string
	Status     string
}

// discoverEnvs globs dir for state files and loads each one's
// TestDefinition via its recorded "_source" entry. A state file with no
// "_source", or whose "_source" YAML no longer parses, is skipped with a
// warning on stderr rather than aborting the whole dashboard — state can
// outlive a YAML that was since moved or deleted.
func discoverEnvs(dir string) ([]discoveredEnv, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.state.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)

	envs := make([]discoveredEnv, 0, len(matches))
	for _, path := range matches {
		st, err := readStateFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			continue
		}
		src, ok := sourcePath(st)
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: no recorded source YAML\n", path)
			continue
		}
		def, err := loadTestDefinition(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: loading %s: %v\n", path, src, err)
			continue
		}
		envs = append(envs, discoveredEnv{
			Def:        def,
			ConfigPath: src,
			StatePath:  path,
			Status:     summarizeStatus(st, def),
		})
	}
	return envs, nil
}

// summarizeStatus renders st's status the same way the dashboard listing
// and the per-env loop's status block do, so the two are always
// consistent: "not provisioned" / "provisioned, agent not installed" / the
// installer's own InstallStatus.Summary (up to date or drifted).
func summarizeStatus(st envState, def TestDefinition) string {
	provisioned, installed := stagesCompleted(st)
	if !provisioned {
		return "not provisioned"
	}
	if !installed {
		return "provisioned, agent not installed"
	}
	status, err := installers.Status(def.Agent.Installer, st, installParamsFor(def))
	if err != nil {
		return fmt.Sprintf("provisioned, agent status unavailable: %v", err)
	}
	return status.Summary
}

// parseIndex parses a 1-based menu choice against n available options,
// returning the 0-based index.
func parseIndex(choice string, n int) (int, bool) {
	i, err := strconv.Atoi(choice)
	if err != nil || i < 1 || i > n {
		return 0, false
	}
	return i - 1, true
}

// runDashboard is e2ectl's no-argument entry point: it lists every
// environment discovered under root's state directory, plus an option to
// open a brand-new config, and enters that environment's loop
// (runEnvLoop, wizard.go) on selection — looping back here once that loop
// returns loopBackToDashboard.
func runDashboard(ctx context.Context, root string) error {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		envs, err := discoverEnvs(stateDirFor(root))
		if err != nil {
			return err
		}

		fmt.Println("\nKnown environments:")
		if len(envs) == 0 {
			fmt.Println("  (none yet)")
		}
		for i, e := range envs {
			fmt.Printf("  %d) %-20s %-45s %s\n", i+1, e.Def.Name, e.Status, e.ConfigPath)
		}
		fmt.Println("  o) open a config...")
		fmt.Println("  q) quit")
		fmt.Print("> ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("reading dashboard selection: %w", err)
			}
			return nil
		}
		choice := strings.TrimSpace(scanner.Text())

		switch {
		case choice == "q":
			return nil
		case choice == "o":
			if err := openConfigAndLoop(ctx, root, scanner); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
		default:
			idx, ok := parseIndex(choice, len(envs))
			if !ok {
				fmt.Println("please enter one of the listed options")
				continue
			}
			e := envs[idx]
			outcome, err := runEnvLoop(ctx, e.Def, e.ConfigPath, e.StatePath, true, scanner)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			if outcome == loopQuit {
				return nil
			}
		}
	}
}

// openConfigAndLoop prompts for a YAML path not yet known to the
// dashboard (no state file exists for it yet — nothing here writes one;
// entering the loop and picking "provision infra" does), loads it, and
// enters its loop the same way a listed environment would.
func openConfigAndLoop(ctx context.Context, root string, scanner *bufio.Scanner) error {
	fmt.Print("path to test definition YAML: ")
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading config path: %w", err)
		}
		return nil
	}
	configPath := strings.TrimSpace(scanner.Text())
	if configPath == "" {
		return nil
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolving absolute path for %s: %w", configPath, err)
	}
	def, err := loadTestDefinition(absConfigPath)
	if err != nil {
		return err
	}
	statePath := defaultStatePath(root, def.Name)

	_, err = runEnvLoop(ctx, def, absConfigPath, statePath, true, scanner)
	return err
}
