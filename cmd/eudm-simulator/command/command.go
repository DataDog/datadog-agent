// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package command provides separate capture and replay command lifecycles.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
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
	Directory string
	Deadline  time.Duration
}

// ReplayRequest owns all verified bytes before delivery can start.
type ReplayRequest struct {
	Scenario      *schema.Scenario
	Plan          *schema.RunPlan
	Bundles       map[string]*bundle.Loaded
	Destinations  map[safety.Destination][]string
	Workers       int
	QueueCapacity int
	DeliveryGrace time.Duration
	ReportPath    string
}

// Runtime separates native capture from the portable replay lifecycle. A replay
// invocation cannot call Capture: only the capture subcommand has that callback.
type Runtime struct {
	Capture func(context.Context, CaptureRequest) error
	Replay  func(context.Context, ReplayRequest) error
}

// MakeCommand constructs the standalone feature-branch command.
func MakeCommand(runtime Runtime) *cobra.Command {
	root := &cobra.Command{Use: "eudm-simulator", Short: "Capture sanitized EUDM baselines and replay staging scenarios", SilenceUsage: true, SilenceErrors: true}
	var request CaptureRequest
	capture := &cobra.Command{Use: "capture", Short: "Capture a native baseline without contacting staging", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if request.Directory == "" || request.Deadline <= 0 {
			return fmt.Errorf("capture requires --output and a positive --deadline")
		}
		if err := nativeCaptureSupported(); err != nil {
			return err
		}
		if runtime.Capture == nil {
			return fmt.Errorf("native capture service is unavailable in this build")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), request.Deadline)
		defer cancel()
		return runtime.Capture(ctx, request)
	}}
	capture.Flags().StringVar(&request.Directory, "output", "", "New sanitized bundle directory")
	capture.Flags().DurationVar(&request.Deadline, "deadline", 35*time.Minute, "Maximum time to obtain required stream coverage")
	root.AddCommand(capture)
	for _, action := range []string{"validate", "plan", "run"} {
		root.AddCommand(replayCommand(action, runtime))
	}
	return root
}

func replayCommand(action string, runtime Runtime) *cobra.Command {
	var scenarioPath, configPath, planPath, outputPath, startText, reportPath string
	var bundleArgs []string
	var seed uint64
	var workers, queueCapacity int
	var deliveryGrace time.Duration
	cmd := &cobra.Command{Use: action, Short: action + " a scenario using separately captured bundles", Args: cobra.NoArgs}
	cmd.Flags().StringVar(&scenarioPath, "scenario", "", "Scenario YAML file")
	cmd.Flags().StringVar(&configPath, "config", "", "Simulator staging configuration YAML file")
	cmd.Flags().StringArrayVar(&bundleArgs, "bundle", nil, "Cohort=directory assignment (repeat for every cohort)")
	if action == "plan" {
		cmd.Flags().Uint64Var(&seed, "seed", 1, "Deterministic variation seed")
		cmd.Flags().StringVar(&startText, "start", "", "Absolute RFC3339 start time (default: one minute from now)")
		cmd.Flags().StringVar(&outputPath, "output", "", "New run-plan JSON file")
	}
	if action == "run" {
		cmd.Flags().StringVar(&planPath, "plan", "", "Previously generated run plan")
		cmd.Flags().IntVar(&workers, "workers", 8, "Bounded concurrency; never limits fleet size")
		cmd.Flags().StringVar(&reportPath, "report", "", "Local report JSON file")
		cmd.Flags().IntVar(&queueCapacity, "queue-capacity", 128, "Bounded replay and process queues; full queues apply backpressure")
		cmd.Flags().DurationVar(&deliveryGrace, "delivery-grace", 5*time.Minute, "Time allowed for normal Agent retries after scenario end")
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if scenarioPath == "" || configPath == "" {
			return fmt.Errorf("--scenario and --config are required")
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
		data, err = os.ReadFile(configPath)
		if err != nil {
			return err
		}
		var config safety.Config
		if err := schema.DecodeStrict(data, &config); err != nil {
			return err
		}
		destinations, err := config.Resolve(os.Getenv)
		if err != nil {
			return err
		}
		assignments, loaded, err := loadAssignments(bundleArgs, &scenario, version.FullCommit)
		if err != nil {
			return err
		}
		start := time.Now().UTC().Add(time.Minute)
		if startText != "" {
			start, err = time.Parse(time.RFC3339, startText)
			if err != nil {
				return fmt.Errorf("--start must be absolute RFC3339: %w", err)
			}
		}
		plan, err := schema.NewPlan(&scenario, scenarioDigest, version.FullCommit, seed, start, assignments)
		if err != nil {
			return err
		}
		if err := engine.Validate(engine.Request{Scenario: &scenario, Plan: plan, Bundles: loaded}); err != nil {
			return err
		}
		switch action {
		case "validate":
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Scenario, staging destinations, and all cohort bundles are valid.")
			return err
		case "plan":
			if outputPath == "" {
				return fmt.Errorf("--output is required")
			}
			return writePlan(outputPath, plan)
		case "run":
			if planPath == "" || reportPath == "" || workers <= 0 || queueCapacity <= 0 || deliveryGrace <= 0 {
				return fmt.Errorf("run requires --plan, --report, and positive --workers, --queue-capacity, and --delivery-grace")
			}
			data, err := os.ReadFile(planPath)
			if err != nil {
				return err
			}
			var persisted schema.RunPlan
			if err := bundle.DecodeJSON(data, &persisted); err != nil {
				return err
			}
			if err := persisted.Validate(&scenario, scenarioDigest, version.FullCommit); err != nil {
				return err
			}
			for _, ref := range persisted.Bundles {
				actual, ok := loaded[ref.Digest]
				if !ok || !reflect.DeepEqual(actual.Ref(), ref) {
					return fmt.Errorf("run plan bundle does not match supplied verified capture")
				}
			}
			if !reflect.DeepEqual(persisted.Assignments, plan.Assignments) {
				return fmt.Errorf("supplied cohort assignments differ from the run plan")
			}
			if !persisted.Start.After(time.Now()) {
				return fmt.Errorf("run-plan start is in the past; generate a new plan")
			}
			if runtime.Replay == nil {
				return fmt.Errorf("portable replay service is unavailable in this build")
			}
			return runtime.Replay(cmd.Context(), ReplayRequest{Scenario: &scenario, Plan: &persisted, Bundles: loaded, Destinations: destinations, Workers: workers, QueueCapacity: queueCapacity, DeliveryGrace: deliveryGrace, ReportPath: reportPath})
		}
		return fmt.Errorf("unsupported action")
	}
	return cmd
}

func loadAssignments(args []string, scenario *schema.Scenario, commit string) (map[string]schema.BundleRef, map[string]*bundle.Loaded, error) {
	refs := map[string]schema.BundleRef{}
	loaded := map[string]*bundle.Loaded{}
	for _, arg := range args {
		group, directory, ok := strings.Cut(arg, "=")
		if !ok || directory == "" || scenario.GroupByName(group) == nil {
			return nil, nil, fmt.Errorf("--bundle must be a declared cohort=directory")
		}
		if _, exists := refs[group]; exists {
			return nil, nil, fmt.Errorf("duplicate --bundle assignment for cohort %q", group)
		}
		capture, err := bundle.Load(directory, commit)
		if err != nil {
			return nil, nil, fmt.Errorf("cohort %q: %w", group, err)
		}
		refs[group] = capture.Ref()
		loaded[capture.Digest] = capture
	}
	for _, group := range scenario.Fleet {
		if _, exists := refs[group.Group]; !exists {
			return nil, nil, fmt.Errorf("cohort %q needs --bundle %s=directory; capture separately on %s, then reuse the bundle", group.Group, group.Group, group.OS)
		}
	}
	return refs, loaded, nil
}

func writePlan(path string, plan *schema.RunPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
