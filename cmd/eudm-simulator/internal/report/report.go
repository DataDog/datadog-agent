// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package report records local expectations and complete fleet delivery accounting.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/identity"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

// Counts tracks complete scheduled collection cycles, including unsent cycles
// after failure. Delivered advances only when every chunk or batch is accepted.
type Counts struct {
	Expected  uint64 `json:"expected"`
	Delivered uint64 `json:"delivered"`
	Failed    uint64 `json:"failed"`
}

// Device accounts for every required stream of one declared device.
type Device struct {
	Ordinal      int                       `json:"ordinal"`
	Hostname     string                    `json:"hostname"`
	Cohort       string                    `json:"cohort"`
	BundleDigest string                    `json:"bundle_digest"`
	Streams      map[schema.Stream]*Counts `json:"streams"`
}

// Phase uses run-relative timing, so reports can be compared across executions.
type Phase struct {
	Name        string        `json:"name"`
	StartOffset time.Duration `json:"start_offset_ns"`
	Duration    time.Duration `json:"duration_ns"`
}

// Report is local-only. The engine owns synchronization and count updates.
type Report struct {
	Version         int                       `json:"version"`
	Status          string                    `json:"status"`
	ScenarioDigest  string                    `json:"scenario_digest"`
	BundleDigests   []string                  `json:"bundle_digests"`
	AgentCommit     string                    `json:"agent_commit"`
	Seed            uint64                    `json:"seed"`
	RunID           string                    `json:"run_id"`
	Start           time.Time                 `json:"start"`
	End             time.Time                 `json:"end"`
	ReplayOS        string                    `json:"replay_os"`
	Phases          []Phase                   `json:"phases"`
	Expectation     schema.Expectation        `json:"expectation"`
	Ledger          []Device                  `json:"ledger"`
	DeclaredDevices int                       `json:"declared_devices"`
	NetworkStreams  map[schema.Stream]*Counts `json:"network_streams"`
	NetworkDevices  []string                  `json:"network_devices"`
	Selectors       map[string]string         `json:"selectors"`
	Errors          []string                  `json:"errors"`
}

// New copies the inputs required to explain and select one run.
func New(plan *schema.RunPlan, scenario *schema.Scenario, replayOS string) *Report {
	r := &Report{Version: 1, Status: "planned", ScenarioDigest: plan.ScenarioDigest, AgentCommit: plan.AgentCommit, Seed: plan.Seed, RunID: plan.RunID, Start: plan.Start, ReplayOS: replayOS, Expectation: schema.Expectation{Conclusion: scenario.Expectation.Conclusion, AffectedCohorts: slices.Clone(scenario.Expectation.AffectedCohorts)}, Selectors: map[string]string{"telemetry": "eudm_run_id:" + plan.RunID, "ndm_namespace": identity.Namespace(plan.RunID)}, Errors: []string{}, NetworkStreams: map[schema.Stream]*Counts{}, NetworkDevices: []string{}}
	for _, assignment := range plan.Assignments {
		r.DeclaredDevices += assignment.Count
	}
	for _, b := range plan.Bundles {
		r.BundleDigests = append(r.BundleDigests, b.Digest)
	}
	var offset time.Duration
	for _, phase := range scenario.Phases {
		r.Phases = append(r.Phases, Phase{Name: phase.Name, StartOffset: offset, Duration: phase.Duration.Duration})
		offset += phase.Duration.Duration
	}
	return r
}

// AddDevice must be called for every declared device before replay starts.
func (r *Report) AddDevice(ordinal int, hostname, cohort, bundleDigest string, streams []schema.Stream) {
	d := Device{Ordinal: ordinal, Hostname: hostname, Cohort: cohort, BundleDigest: bundleDigest, Streams: map[schema.Stream]*Counts{}}
	for _, stream := range streams {
		d.Streams[stream] = &Counts{}
	}
	r.Ledger = append(r.Ledger, d)
}

// Complete is false if any required stream is missing, failed, or undelivered.
func (r *Report) Complete() bool {
	if len(r.Errors) > 0 || len(r.Ledger) == 0 || len(r.Ledger) != r.DeclaredDevices || r.End.IsZero() {
		return false
	}
	seen := make(map[int]bool, len(r.Ledger))
	for _, device := range r.Ledger {
		if device.Ordinal < 0 || device.Ordinal >= r.DeclaredDevices || seen[device.Ordinal] || device.Hostname == "" || len(device.Streams) == 0 {
			return false
		}
		seen[device.Ordinal] = true
		for _, counts := range device.Streams {
			if counts == nil || counts.Expected == 0 || counts.Failed != 0 || counts.Delivered != counts.Expected {
				return false
			}
		}
	}
	for _, counts := range r.NetworkStreams {
		if counts == nil || counts.Expected == 0 || counts.Failed != 0 || counts.Delivered != counts.Expected {
			return false
		}
	}
	return true
}

// Write creates a private local report and refuses to overwrite prior evidence.
func (r *Report) Write(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local run report: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
