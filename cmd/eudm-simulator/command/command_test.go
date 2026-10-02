// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/bazelbuild/rules_go/go/runfiles"
)

func TestReplayCommandsReadSiteFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	scenario := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenario, []byte("version: 1\nscenario: {name: healthy}\nexpectation: {conclusion: healthy}\nfleet: [{group: mac, os: macos, count: 5}]\nphases: [{name: healthy, duration: 20m}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DD_API_KEY", "")
	for _, action := range []string{"validate", "run"} {
		for _, tc := range []struct {
			name string
			site string
			want string
		}{
			{name: "staging", site: "datad0g.com", want: "--bundle is required"},
			{name: "missing", want: "DD_SITE must be"},
			{name: "production", site: "datadoghq.com", want: "DD_SITE must be"},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				t.Setenv("DD_SITE", tc.site)
				called := false
				cmd := MakeCommand(Runtime{Capture: func(context.Context, CaptureRequest) error { called = true; return nil }, Replay: func(context.Context, ReplayRequest) error { called = true; return nil }})
				cmd.SetArgs([]string{action, "--scenario", scenario})
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("expected %q, got %v", tc.want, err)
				}
				if called {
					t.Fatal("missing evidence or invalid site started capture or delivery")
				}
			})
		}
	}
}

func TestCommandSurface(t *testing.T) {
	cmd := MakeCommand(Runtime{})
	for _, name := range []string{"capture", "validate", "run"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil || sub == cmd {
			t.Fatalf("missing command %s", name)
		}
		if sub.Flags().Lookup("fast") != nil {
			t.Fatal("staging time acceleration is exposed")
		}
		for _, flag := range []string{"cfgpath", "duration", "timeout"} {
			if (sub.Flags().Lookup(flag) != nil) != (name == "capture") {
				t.Fatalf("--%s must be available only to capture", flag)
			}
		}
		for _, flag := range []string{"config", "plan", "start", "workers", "queue-capacity", "delivery-grace", "deadline"} {
			if sub.Flags().Lookup(flag) != nil {
				t.Fatalf("%s still exposes the removed --%s option", name, flag)
			}
		}
	}
}

func TestCaptureRequiresDurationAndDefaultsDeadlineToDurationPlusGrace(t *testing.T) {
	if err := captureSupported(); err != nil {
		t.Skip(err)
	}
	t.Setenv("DD_SITE", "")
	t.Setenv("DD_API_KEY", "")
	directory := filepath.Join(t.TempDir(), "capture")
	cfgpath := filepath.Join(t.TempDir(), "datadog.yaml")
	before := time.Now()
	var output bytes.Buffer
	var captureContext context.Context
	cmd := MakeCommand(Runtime{
		Capture: func(ctx context.Context, request CaptureRequest) error {
			captureContext = ctx
			if request.Progress != &output {
				t.Fatal("capture did not receive command output")
			}
			if request.Directory != directory {
				t.Fatalf("unexpected capture directory: %s", request.Directory)
			}
			if request.ConfigPath != cfgpath {
				t.Fatal("capture did not receive its installed configuration path")
			}
			deadline, ok := ctx.Deadline()
			if request.Duration != time.Hour || !ok || deadline.Before(before.Add(65*time.Minute)) || deadline.After(time.Now().Add(65*time.Minute)) {
				t.Fatalf("capture must allow its full duration plus setup/cleanup, got %v", deadline)
			}
			return nil
		},
		Replay: func(context.Context, ReplayRequest) error {
			t.Fatal("capture started delivery")
			return nil
		},
	})
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"capture", "--output", directory, "--cfgpath", cfgpath, "--duration", "1h"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if captureContext == nil || captureContext.Err() != context.Canceled {
		t.Fatal("capture did not release its timeout after completion")
	}
}

func TestReplayRejectsCaptureConfig(t *testing.T) {
	for _, action := range []string{"run", "validate"} {
		cmd := MakeCommand(Runtime{})
		cmd.SetArgs([]string{action, "--cfgpath", "installed-agent-config"})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag: --cfgpath") {
			t.Fatalf("replay accepted installed configuration: %v", err)
		}
	}
}

func TestReplayBundleIsALiteralBaselinePath(t *testing.T) {
	dir := t.TempDir()
	scenario := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenario, []byte("version: 1\nscenario: {name: healthy}\nexpectation: {conclusion: healthy}\nfleet: [{group: baseline, os: macos, count: 5}]\nphases: [{name: healthy, duration: 20m}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DD_SITE", "datad0g.com")
	for _, action := range []string{"validate", "run"} {
		for _, path := range []string{filepath.Join(dir, "capture"), "baseline=" + filepath.Join(dir, "capture")} {
			t.Run(action+"/"+filepath.Base(path), func(t *testing.T) {
				cmd := MakeCommand(Runtime{})
				cmd.SetArgs([]string{action, "--scenario", scenario, "--bundle", path})
				err := cmd.Execute()
				var pathErr *os.PathError
				if !errors.As(err, &pathErr) || pathErr.Path != path {
					t.Fatalf("expected baseline path %q to be opened literally, got %v", path, err)
				}
			})
		}
	}
}

const directRunScenario = `version: 1
scenario: {name: healthy}
expectation: {conclusion: healthy}
fleet:
  - {group: baseline, os: macos, count: 2}
  - {group: comparison, os: macos, count: 3}
phases:
  - name: healthy
    duration: 30s
`

func replayFixture(t *testing.T) (scenarioPath, bundlePath string) {
	t.Helper()
	previousCommit := version.FullCommit
	version.FullCommit = strings.Repeat("a", 40)
	t.Cleanup(func() { version.FullCommit = previousCommit })
	t.Setenv("DD_SITE", "datad0g.com")

	scenarioPath = filepath.Join(t.TempDir(), "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(directRunScenario), 0600); err != nil {
		t.Fatal(err)
	}
	fixturePath := func(name string) string {
		relative := "testdata/bundles/macos/" + name
		if os.Getenv("TEST_SRCDIR") == "" {
			return filepath.Join("..", filepath.FromSlash(relative))
		}
		files, err := runfiles.New()
		if err != nil {
			t.Fatal(err)
		}
		repository := runfiles.CallerRepository()
		if repository == "" {
			repository = "_main"
		}
		path, err := files.Rlocation(repository + "/cmd/eudm-simulator/" + relative)
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	data, err := os.ReadFile(fixturePath("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest bundle.Manifest
	if err := bundle.DecodeJSON(data, &manifest); err != nil {
		t.Fatal(err)
	}
	// The loader requires regular files, while Bazel provides fixture symlinks.
	bundlePath = t.TempDir()
	names := []string{"manifest.json", "COMPLETE"}
	for name := range manifest.Files {
		names = append(names, name)
	}
	for _, name := range names {
		data, err := os.ReadFile(fixturePath(name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundlePath, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return scenarioPath, bundlePath
}

func TestRunDirectlyFromScenarioAndBaseline(t *testing.T) {
	scenarioPath, bundlePath := replayFixture(t)
	explicitReport := filepath.Join(t.TempDir(), "report.json")
	for _, tc := range []struct {
		name   string
		args   []string
		seed   uint64
		report string
	}{
		{name: "defaults", seed: 1},
		{name: "explicit options", args: []string{"--seed", "42", "--report", explicitReport}, seed: 42, report: explicitReport},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got *ReplayRequest
			cmd := MakeCommand(Runtime{
				Capture: func(context.Context, CaptureRequest) error {
					t.Fatal("run started live capture")
					return nil
				},
				Replay: func(_ context.Context, request ReplayRequest) error {
					if got != nil {
						t.Fatal("replay started more than once")
					}
					got = &request
					return nil
				},
			})
			cmd.SetArgs(append([]string{"run", "--scenario", scenarioPath, "--bundle", bundlePath}, tc.args...))
			before := time.Now().UTC()
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if got == nil || got.Bundle == nil || got.Plan == nil || got.Scenario == nil {
				t.Fatal("direct run did not provide verified replay inputs")
			}
			if !reflect.DeepEqual(got.Plan.Bundle, got.Bundle.Ref()) || got.Plan.AgentCommit != version.FullCommit {
				t.Fatal("run metadata does not reference the verified baseline")
			}
			if len(got.Scenario.Fleet) != 2 || got.Scenario.Fleet[0].Count != 2 || got.Scenario.Fleet[1].Count != 3 {
				t.Fatal("baseline was not retained for the complete scenario fleet")
			}
			if got.Plan.Seed != tc.seed || len(got.Plan.RunID) != 32 || got.Plan.Start.Before(before) || got.Plan.Start.After(time.Now()) {
				t.Fatalf("unexpected direct-run identity, seed, or start: %+v", got.Plan)
			}
			wantReport := tc.report
			if wantReport == "" {
				wantReport = "eudm-run-" + got.Plan.RunID + ".json"
			}
			if got.ReportPath != wantReport {
				t.Fatalf("unexpected direct-run options: %+v", got)
			}
		})
	}
}

func TestRunShowsProgressAndFinalResult(t *testing.T) {
	scenarioPath, bundlePath := replayFixture(t)
	for _, failed := range []bool{false, true} {
		t.Run(strconv.FormatBool(failed), func(t *testing.T) {
			var output bytes.Buffer
			var runErr error
			status := "succeeded"
			if failed {
				runErr = errors.New("injected delivery failure")
				status = "failed"
			}
			path := filepath.Join(t.TempDir(), "report.json")
			cmd := MakeCommand(Runtime{Replay: func(_ context.Context, request ReplayRequest) error {
				if request.Progress == nil {
					t.Fatal("replay did not receive the command output writer")
				}
				if _, err := fmt.Fprintln(request.Progress, "Replay 30s/35m0s | phase=healthy | replaying"); err != nil {
					t.Fatal(err)
				}
				return runErr
			}})
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"run", "--scenario", scenarioPath, "--bundle", bundlePath, "--report", path})
			if err := cmd.Execute(); !errors.Is(err, runErr) {
				t.Fatalf("run error was lost: %v", err)
			}
			want := "Run report: " + path + "\nReplay 30s/35m0s | phase=healthy | replaying\nRun " + status + ". Report: " + path + "\n"
			if output.String() != want {
				t.Fatalf("missing progress or final result: %s", output.String())
			}
		})
	}
}

func TestValidateBaselineNeedsNoAPIKeyAndDoesNotReplay(t *testing.T) {
	scenarioPath, bundlePath := replayFixture(t)
	t.Setenv("DD_API_KEY", "")
	cmd := MakeCommand(Runtime{
		Capture: func(context.Context, CaptureRequest) error {
			t.Fatal("validate started live capture")
			return nil
		},
		Replay: func(context.Context, ReplayRequest) error {
			t.Fatal("validate started delivery")
			return nil
		},
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"validate", "--scenario", scenarioPath, "--bundle", bundlePath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "baseline capture are valid") {
		t.Fatalf("missing validation result: %s", output.String())
	}
}

func TestReplayRejectsBaselineMissingScenarioEvidence(t *testing.T) {
	scenarioPath, bundlePath := replayFixture(t)
	scenario := directRunScenario + "    metrics:\n      comparison:\n        system.disk.utilized: {steady: {value: 1}}\n"
	if err := os.WriteFile(scenarioPath, []byte(scenario), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"validate", "run"} {
		t.Run(action, func(t *testing.T) {
			cmd := MakeCommand(Runtime{Replay: func(context.Context, ReplayRequest) error {
				t.Fatal("missing scenario evidence started delivery")
				return nil
			}})
			cmd.SetArgs([]string{action, "--scenario", scenarioPath, "--bundle", bundlePath})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "comparison") || !strings.Contains(err.Error(), "system.disk.utilized") || !strings.Contains(err.Error(), "was not captured") {
				t.Fatalf("expected missing evidence for the second group, got %v", err)
			}
		})
	}
}

func TestRemovedCommandAndFlagsAreRejected(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"plan"}, want: `unknown command "plan"`},
		{args: []string{"run", "--plan", "plan.json"}, want: "unknown flag: --plan"},
		{args: []string{"run", "--start", "2026-09-29T12:00:00Z"}, want: "unknown flag: --start"},
		{args: []string{"run", "--config", "staging.yaml"}, want: "unknown flag: --config"},
		{args: []string{"run", "--workers", "3"}, want: "unknown flag: --workers"},
		{args: []string{"run", "--queue-capacity", "7"}, want: "unknown flag: --queue-capacity"},
		{args: []string{"run", "--delivery-grace", "1m"}, want: "unknown flag: --delivery-grace"},
		{args: []string{"capture", "--deadline", "35m"}, want: "unknown flag: --deadline"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := MakeCommand(Runtime{})
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCaptureAcceptsBoundedTimeoutForHourlyHardware(t *testing.T) {
	if captureSupported() != nil {
		t.Skip("live capture requires macOS or Windows")
	}
	for _, timeout := range []string{"70m", "0s", "-1s", "60m", "126m"} {
		t.Run(timeout, func(t *testing.T) {
			called := false
			cmd := MakeCommand(Runtime{Capture: func(ctx context.Context, _ CaptureRequest) error {
				called = true
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) < 69*time.Minute || time.Until(deadline) > 70*time.Minute {
					t.Fatal("custom capture deadline lost")
				}
				return nil
			}})
			cmd.SetArgs([]string{"capture", "--output", filepath.Join(t.TempDir(), "capture"), "--duration", "1h", "--timeout", timeout})
			err := cmd.Execute()
			if timeout == "70m" {
				if err != nil || !called {
					t.Fatalf("hourly capture timeout rejected: %v", err)
				}
			} else if err == nil || called {
				t.Fatal("invalid timeout started capture")
			}
		})
	}
}

func TestCaptureRejectsMissingOrInvalidDuration(t *testing.T) {
	for _, duration := range []string{"", "0s", "-1s", "121m"} {
		t.Run(duration, func(t *testing.T) {
			cmd := MakeCommand(Runtime{Capture: func(context.Context, CaptureRequest) error {
				t.Fatal("invalid recording duration started capture")
				return nil
			}})
			args := []string{"capture", "--output", filepath.Join(t.TempDir(), "capture")}
			if duration != "" {
				args = append(args, "--duration", duration)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--duration") {
				t.Fatalf("expected duration error, got %v", err)
			}
		})
	}
}

func TestReplayRejectsScenarioLongerThanRecording(t *testing.T) {
	scenarioPath, bundlePath := replayFixture(t)
	scenario := strings.Replace(directRunScenario, "duration: 30s", "duration: 6m", 1)
	if err := os.WriteFile(scenarioPath, []byte(scenario), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"validate", "run"} {
		t.Run(action, func(t *testing.T) {
			cmd := MakeCommand(Runtime{Replay: func(context.Context, ReplayRequest) error {
				t.Fatal("short recording reached delivery")
				return nil
			}})
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{action, "--scenario", scenarioPath, "--bundle", bundlePath})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "6m0s") || !strings.Contains(err.Error(), "5m1s") || !strings.Contains(err.Error(), "--duration") {
				t.Fatalf("missing required/available duration and capture guidance: %v", err)
			}
			if output.Len() != 0 {
				t.Fatal("short recording announced a run")
			}
		})
	}
}
