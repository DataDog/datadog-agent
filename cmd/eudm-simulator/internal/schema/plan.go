// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// BundleRef records content identity, never a replay-host-specific path.
type BundleRef struct {
	Digest            string        `json:"digest"`
	CaptureToolCommit string        `json:"capture_tool_commit"`
	Duration          time.Duration `json:"duration_ns"`
	Profile           Profile       `json:"profile"`
}

// RunPlanVersion identifies the in-memory single-baseline execution contract.
const RunPlanVersion = 6

// RunPlan holds in-memory execution metadata. Each run creates it directly from
// the scenario and baseline; Execute establishes Start after startup completes.
type RunPlan struct {
	Version        int       `json:"version"`
	ScenarioDigest string    `json:"scenario_digest"`
	RunID          string    `json:"run_id"`
	Seed           uint64    `json:"seed"`
	Start          time.Time `json:"start"`
	AgentCommit    string    `json:"agent_commit"`
	Bundle         BundleRef `json:"bundle"`
}

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var runIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Digest computes the content identity of a file, including its exact encoding.
func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// NewPlan binds every scenario group to the same baseline capture.
func NewPlan(s *Scenario, scenarioDigest, commit string, seed uint64, start time.Time, baseline BundleRef) (*RunPlan, error) {
	p := &RunPlan{Version: RunPlanVersion, ScenarioDigest: scenarioDigest, AgentCommit: commit, Seed: seed, Start: start.UTC(), Bundle: baseline}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	p.RunID = hex.EncodeToString(id[:])
	return p, p.Validate(s, scenarioDigest, commit)
}

// Validate checks execution metadata against the current scenario and binary.
// The bundle loader verifies the referenced file digests before calling this.
func (p *RunPlan) Validate(s *Scenario, scenarioDigest, commit string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if p.Version != RunPlanVersion {
		return fmt.Errorf("unsupported run-plan version %d", p.Version)
	}
	if !digestPattern.MatchString(p.ScenarioDigest) || p.ScenarioDigest != scenarioDigest {
		return errors.New("run plan scenario digest mismatch")
	}
	if !commitPattern.MatchString(commit) || p.AgentCommit != commit {
		return errors.New("run plan requires the current simulator commit; rebuild the simulator")
	}
	if !runIDPattern.MatchString(p.RunID) || p.Start.IsZero() {
		return errors.New("run plan requires an opaque run ID and absolute start time")
	}
	if !digestPattern.MatchString(p.Bundle.Digest) || !commitPattern.MatchString(p.Bundle.CaptureToolCommit) {
		return errors.New("bundle requires valid content and capture-tool identities")
	}
	if p.Bundle.Duration <= 0 {
		return errors.New("bundle requires a positive recording duration")
	}
	var duration time.Duration
	for _, phase := range s.Phases {
		duration += phase.Duration.Duration
	}
	if duration > p.Bundle.Duration {
		return fmt.Errorf("scenario requires %s of recorded telemetry, but the bundle contains %s; recapture with --duration at least %s", duration, p.Bundle.Duration, duration)
	}
	for _, group := range s.Fleet {
		if err := s.ValidateEvidence(group, p.Bundle.Profile); err != nil {
			return err
		}
	}
	return nil
}
