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
	Digest      string  `json:"digest"`
	AgentCommit string  `json:"agent_commit"`
	Profile     Profile `json:"profile"`
}

// Assignment binds one complete cohort to one exact capture.
type Assignment struct {
	Cohort       string `json:"cohort"`
	BundleDigest string `json:"bundle_digest"`
	FirstOrdinal int    `json:"first_ordinal"`
	Count        int    `json:"count"`
}

// RunPlan is a portable, immutable description of one execution.
type RunPlan struct {
	Version        int          `json:"version"`
	ScenarioDigest string       `json:"scenario_digest"`
	RunID          string       `json:"run_id"`
	Seed           uint64       `json:"seed"`
	Start          time.Time    `json:"start"`
	AgentCommit    string       `json:"agent_commit"`
	Bundles        []BundleRef  `json:"bundles"`
	Assignments    []Assignment `json:"assignments"`
}

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var runIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Digest computes the content identity of a file, including its exact encoding.
func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// NewPlan validates every cohort before allocating a random opaque run ID.
// assignments must be explicit, even if there is only one matching bundle.
func NewPlan(s *Scenario, scenarioDigest, commit string, seed uint64, start time.Time, assignments map[string]BundleRef) (*RunPlan, error) {
	p := &RunPlan{Version: Version, ScenarioDigest: scenarioDigest, AgentCommit: commit, Seed: seed, Start: start.UTC()}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	p.RunID = hex.EncodeToString(id[:])
	seen := map[string]bool{}
	ordinal := 0
	if len(assignments) != len(s.Fleet) {
		return nil, errors.New("supply exactly one --bundle cohort=directory for every declared cohort; capture runs separately")
	}
	for _, group := range s.Fleet {
		bundle, ok := assignments[group.Group]
		if !ok {
			return nil, fmt.Errorf("cohort %q needs --bundle %s=directory; capture a compatible bundle separately on %s", group.Group, group.Group, group.OS)
		}
		if !seen[bundle.Digest] {
			p.Bundles = append(p.Bundles, bundle)
			seen[bundle.Digest] = true
		}
		p.Assignments = append(p.Assignments, Assignment{Cohort: group.Group, BundleDigest: bundle.Digest, FirstOrdinal: ordinal, Count: group.Count})
		ordinal += group.Count
	}
	return p, p.Validate(s, scenarioDigest, commit)
}

// Validate rechecks a persisted plan against the current scenario and binary.
// The bundle loader verifies the referenced file digests before calling this.
func (p *RunPlan) Validate(s *Scenario, scenarioDigest, commit string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if p.Version != Version {
		return fmt.Errorf("unsupported run-plan version %d", p.Version)
	}
	if !digestPattern.MatchString(p.ScenarioDigest) || p.ScenarioDigest != scenarioDigest {
		return errors.New("run plan scenario digest mismatch")
	}
	if !commitPattern.MatchString(commit) || p.AgentCommit != commit {
		return errors.New("run plan requires the exact Agent commit; rebuild and recapture bundles")
	}
	if !runIDPattern.MatchString(p.RunID) || p.Start.IsZero() {
		return errors.New("run plan requires an opaque run ID and absolute start time")
	}
	refs := map[string]BundleRef{}
	for _, ref := range p.Bundles {
		if !digestPattern.MatchString(ref.Digest) || ref.AgentCommit != commit {
			return errors.New("bundle digest or Agent commit mismatch; recapture with this Agent revision")
		}
		if _, exists := refs[ref.Digest]; exists {
			return errors.New("duplicate bundle digest")
		}
		refs[ref.Digest] = ref
	}
	if len(p.Assignments) != len(s.Fleet) {
		return errors.New("run plan does not cover every cohort")
	}
	ordinal := 0
	used := map[string]bool{}
	for i, group := range s.Fleet {
		a := p.Assignments[i]
		if a.Cohort != group.Group || a.Count != group.Count || a.FirstOrdinal != ordinal {
			return fmt.Errorf("run plan cohort %q count, order, or ordinal mismatch", group.Group)
		}
		ref, exists := refs[a.BundleDigest]
		if !exists {
			return fmt.Errorf("cohort %q has no assigned compatible bundle", group.Group)
		}
		if err := s.ValidateEvidence(group, ref.Profile); err != nil {
			return err
		}
		used[a.BundleDigest] = true
		ordinal += group.Count
	}
	if len(used) != len(refs) {
		return errors.New("run plan contains unused bundles")
	}
	return nil
}
