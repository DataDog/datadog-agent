// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package command provides separate capture and replay command lifecycles.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/engine"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// CaptureRequest never carries delivery credentials or staging destinations.
type CaptureRequest struct {
	Directory  string
	ConfigPath string
	Duration   time.Duration
	Progress   io.Writer
}

// ReplayRequest owns all verified bytes before delivery can start.
type ReplayRequest struct {
	Scenario     *schema.Scenario
	Plan         *schema.RunPlan
	Bundle       *bundle.Loaded
	Destinations map[safety.Destination][]string
	ReportPath   string
	Progress     io.Writer
}

// Runtime separates live capture from the portable replay lifecycle. A replay
// invocation cannot call Capture: only the capture subcommand has that callback.
type Runtime struct {
	Capture func(context.Context, CaptureRequest) error
	Replay  func(context.Context, ReplayRequest) error
}

const captureSetupGrace = 5 * time.Minute

// MakeCommand constructs the standalone feature-branch command.
func MakeCommand(runtime Runtime) *cobra.Command {
	root := &cobra.Command{Use: "eudm-simulator", Short: "Capture Agent telemetry and replay staging scenarios", SilenceUsage: true, SilenceErrors: true}
	var request CaptureRequest
	var timeout time.Duration
	capture := &cobra.Command{Use: "capture", Short: "Capture output from running Agent services", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if request.Directory == "" {
			return errors.New("capture requires --output")
		}
		if request.Duration <= 0 || request.Duration > 2*time.Hour {
			return errors.New("capture requires --duration greater than zero and at most 2h")
		}
		if err := captureSupported(); err != nil {
			return err
		}
		if runtime.Capture == nil {
			return errors.New("live capture coordinator is unavailable in this build")
		}
		deadline := timeout
		if !cmd.Flags().Changed("timeout") {
			deadline = request.Duration + captureSetupGrace
		}
		if deadline <= request.Duration || deadline > 2*time.Hour+captureSetupGrace {
			return errors.New("capture --timeout must exceed --duration and be at most 2h5m")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), deadline)
		defer cancel()
		request.Progress = cmd.OutOrStdout()
		return runtime.Capture(ctx, request)
	}}
	capture.Flags().DurationVar(&request.Duration, "duration", 0, "Required recording duration (up to 2h; must cover the scenario and required collection schedules)")
	capture.Flags().DurationVar(&timeout, "timeout", 0, "Overall deadline including setup and cleanup (default: duration plus 5m; maximum 2h5m)")
	capture.Flags().StringVar(&request.Directory, "output", "", "New telemetry bundle directory")
	capture.Flags().StringVar(&request.ConfigPath, "cfgpath", "", "Installed Agent configuration file or directory (capture APIs and authentication only)")
	root.AddCommand(capture)
	for _, action := range []string{"validate", "run"} {
		root.AddCommand(replayCommand(action, runtime))
	}
	return root
}

func replayCommand(action string, runtime Runtime) *cobra.Command {
	var scenarioPath, bundlePath, reportPath string
	seed := uint64(1)
	cmd := &cobra.Command{Use: action, Short: action + " a scenario using one baseline capture", Args: cobra.NoArgs}
	cmd.Flags().StringVar(&scenarioPath, "scenario", "", "Scenario YAML file")
	cmd.Flags().StringVar(&bundlePath, "bundle", "", "Baseline capture directory used by every scenario group")
	if action == "run" {
		cmd.Flags().Uint64Var(&seed, "seed", 1, "Deterministic variation seed")
		cmd.Flags().StringVar(&reportPath, "report", "", "Local report JSON file (default: eudm-run-<run-id>.json)")
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if scenarioPath == "" {
			return errors.New("--scenario is required")
		}
		data, err := os.ReadFile(scenarioPath)
		if err != nil {
			return err
		}
		var scenario schema.Scenario
		if err := schema.DecodeStrict(data, &scenario); err != nil {
			return err
		}
		if err := scenario.Validate(); err != nil {
			return err
		}
		scenarioDigest := schema.Digest(data)
		config := safety.Config{Site: os.Getenv("DD_SITE")}
		destinations, err := config.Resolve(os.Getenv)
		if err != nil {
			return err
		}
		if bundlePath == "" {
			return errors.New("--bundle is required: supply the baseline capture directory")
		}
		loaded, err := bundle.Load(bundlePath, version.FullCommit)
		if err != nil {
			return err
		}
		plan, err := schema.NewPlan(&scenario, scenarioDigest, version.FullCommit, seed, time.Now().UTC(), loaded.Ref())
		if err != nil {
			return err
		}
		switch action {
		case "validate":
			if err := engine.Validate(engine.Request{Scenario: &scenario, Plan: plan, Bundle: loaded}); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Scenario, staging destinations, and baseline capture are valid.")
			return err
		case "run":
			if runtime.Replay == nil {
				return errors.New("portable replay service is unavailable in this build")
			}
			if reportPath == "" {
				reportPath = fmt.Sprintf("eudm-run-%s.json", plan.RunID)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Run report: %s\n", reportPath); err != nil {
				return err
			}
			runErr := runtime.Replay(cmd.Context(), ReplayRequest{Scenario: &scenario, Plan: plan, Bundle: loaded, Destinations: destinations, ReportPath: reportPath, Progress: cmd.OutOrStdout()})
			status := "succeeded"
			if runErr != nil {
				status = "failed"
			}
			_, outputErr := fmt.Fprintf(cmd.OutOrStdout(), "Run %s. Report: %s\n", status, reportPath)
			return errors.Join(runErr, outputErr)
		}
		return errors.New("unsupported action")
	}
	return cmd
}
