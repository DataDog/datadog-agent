// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

const healthyYAML = `version: 1
scenario: {name: healthy-mixed}
expectation: {conclusion: healthy, affected_cohorts: []}
fleet:
  - {group: mac, os: macos, count: 3}
  - {group: win, os: windows, count: 4}
phases:
  - {name: healthy, duration: 20m}
`

func healthy(t *testing.T) *Scenario {
	t.Helper()
	var scenario Scenario
	if err := DecodeStrict([]byte(healthyYAML), &scenario); err != nil {
		t.Fatal(err)
	}
	return &scenario
}

func refs() map[string]BundleRef {
	result := map[string]BundleRef{}
	for group, platform := range map[string]string{"mac": "macos", "win": "windows"} {
		result[group] = BundleRef{Digest: Digest([]byte(platform)), AgentCommit: strings.Repeat("a", 40), Profile: Profile{OS: platform, Architecture: "arm64", Streams: []Stream{Metrics, HostMetadata, Processes, Software}, MetricNames: []string{"system.cpu.user"}, ProcessNames: []string{"Chrome"}, SoftwareNames: []string{"Google Chrome"}}}
	}
	return result
}

func TestMixedPlatformPlanRoundTrip(t *testing.T) {
	s := healthy(t)
	digest, commit := Digest([]byte(healthyYAML)), strings.Repeat("a", 40)
	p, err := NewPlan(s, digest, commit, 42, time.Date(2026, 9, 29, 1, 2, 3, 0, time.UTC), refs())
	if err != nil {
		t.Fatal(err)
	}
	if p.Assignments[0].FirstOrdinal != 0 || p.Assignments[1].FirstOrdinal != 3 || p.Assignments[1].Count != 4 {
		t.Fatal("fleet was truncated or reordered", p.Assignments)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored RunPlan
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(s, digest, commit); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.RunID, s.Meta.Name) || len(p.RunID) != 32 {
		t.Fatal("nonopaque identity")
	}
}

func TestPlanRejectsMissingAndIncompatibleEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Scenario, map[string]BundleRef)
	}{
		{"missing cohort", func(_ *Scenario, r map[string]BundleRef) { delete(r, "mac") }},
		{"OS mismatch", func(_ *Scenario, r map[string]BundleRef) { r["mac"] = r["win"] }},
		{"missing stream", func(_ *Scenario, r map[string]BundleRef) {
			b := r["win"]
			b.Profile.Streams = []Stream{Metrics}
			r["win"] = b
		}},
		{"wrong commit", func(_ *Scenario, r map[string]BundleRef) {
			b := r["win"]
			b.AgentCommit = strings.Repeat("b", 40)
			r["win"] = b
		}},
		{"invented hardware", func(s *Scenario, _ map[string]BundleRef) { s.Fleet[0].TotalRAMGB = 128 }},
		{"invented metric", func(s *Scenario, _ map[string]BundleRef) {
			s.Phases[0].Metrics = map[string]map[string]Pattern{"mac": {"system.wlan.rssi": {Steady: &SteadyPattern{Value: -55}}}}
		}},
		{"invented process", func(s *Scenario, _ map[string]BundleRef) {
			s.Phases[0].Processes = map[string][]ProcessDef{"win": {{Name: "not-captured", CPU: Pattern{Steady: &SteadyPattern{Value: 1}}, Memory: Pattern{Steady: &SteadyPattern{Value: 20}}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := healthy(t), refs()
			tc.mutate(s, r)
			if _, err := NewPlan(s, Digest([]byte(healthyYAML)), strings.Repeat("a", 40), 1, time.Now(), r); err == nil {
				t.Fatal("accepted invalid evidence")
			}
		})
	}
}

func TestStrictScenarioRejectsUnsupportedEvidence(t *testing.T) {
	for _, suffix := range []string{"archive: {}", "events: {}", "redapl: {}", "unknown: true", "---\nversion: 1"} {
		var scenario Scenario
		if err := DecodeStrict([]byte(healthyYAML+suffix+"\n"), &scenario); err == nil {
			t.Errorf("accepted %s", suffix)
		}
	}
	for _, pattern := range []string{"{steady: {value: 2, secret: x}}", "{ramp: {from: 1, to: 3, typo: 2}}", "{step: {at: 5, before: 1, after: 2, extra: 1}}"} {
		var p Pattern
		if err := DecodeStrict([]byte(pattern), &p); err == nil {
			t.Errorf("accepted unknown pattern field: %s", pattern)
		}
	}
	s := healthy(t)
	s.Fleet[0].OS = "linux"
	if err := s.Validate(); err == nil {
		t.Fatal("accepted Linux device")
	}
}

func TestConnectionSelectorsRequireCapturedStream(t *testing.T) {
	s := healthy(t)
	s.Phases[0].Connections = map[string][]ConnectionOverlay{"win": {{Selector: "vpn-1", RTTMilliseconds: &Pattern{Steady: &SteadyPattern{Value: 400}}, TCPFailures: map[uint32]Pattern{110: {Steady: &SteadyPattern{Value: 1}}}}}}
	r := refs()
	b := r["win"]
	if err := s.ValidateEvidence(s.Fleet[1], b.Profile); err == nil {
		t.Fatal("accepted uncaptured connections")
	}
	b.Profile.Streams = append(b.Profile.Streams, Connections)
	if err := s.ValidateEvidence(s.Fleet[1], b.Profile); err == nil {
		t.Fatal("accepted uncaptured selector")
	}
	b.Profile.ConnectionSelectors = []string{"vpn-1"}
	if err := s.ValidateEvidence(s.Fleet[1], b.Profile); err != nil {
		t.Fatal(err)
	}
	s.Phases[0].Connections["win"][0].TCPFailures = map[uint32]Pattern{10060: {Steady: &SteadyPattern{Value: 1}}}
	if err := s.Validate(); err == nil {
		t.Fatal("accepted platform-native rather than standardized failure code")
	}
}

func TestInvalidExpectationsPhasesAndPatterns(t *testing.T) {
	for _, mutate := range []func(*Scenario){
		func(s *Scenario) { s.Expectation.Conclusion = "invented" },
		func(s *Scenario) { s.Expectation.AffectedCohorts = []string{"mac"} },
		func(s *Scenario) { s.Fleet[0].Tags = []string{"affected:true"} },
		func(s *Scenario) { s.Fleet[0].Tags = []string{"test:healthy-mixed"} },
		func(s *Scenario) { s.Fleet[0].BaselineVariance = math.NaN() },
		func(s *Scenario) { s.Phases[0].Duration.Duration = 0 },
		func(s *Scenario) {
			s.Phases[0].Metrics = map[string]map[string]Pattern{"mac": {"system.cpu.user": {Steady: &SteadyPattern{Value: math.Inf(1)}}}}
		},
	} {
		s := healthy(t)
		mutate(s)
		if err := s.Validate(); err == nil {
			t.Fatal("accepted invalid scenario")
		}
	}
	s := healthy(t)
	s.Expectation = Expectation{Conclusion: VPNPath, AffectedCohorts: []string{"win"}}
	s.MonitorWindow.Duration = 10 * time.Minute
	s.VisibilityDelay.Duration = 5 * time.Minute
	s.Phases = nil
	for _, name := range []string{"healthy", "onset", "sustained", "recovery"} {
		s.Phases = append(s.Phases, Phase{Name: name, Duration: Duration{20 * time.Minute}})
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.Phases[2].Duration.Duration = 10 * time.Minute
	if err := s.Validate(); err == nil {
		t.Fatal("accepted insufficient sustained monitor window")
	}
}

func TestPersistedPlanTampering(t *testing.T) {
	for _, mutate := range []func(*RunPlan){
		func(p *RunPlan) { p.Version++ }, func(p *RunPlan) { p.ScenarioDigest = Digest([]byte("changed")) },
		func(p *RunPlan) { p.RunID = "healthy-mixed" }, func(p *RunPlan) { p.Start = time.Time{} },
		func(p *RunPlan) { p.Assignments[1].Count-- }, func(p *RunPlan) { p.Assignments[1].FirstOrdinal = 0 },
		func(p *RunPlan) { p.Bundles[0].AgentCommit = strings.Repeat("b", 40) },
	} {
		s := healthy(t)
		digest, commit := Digest([]byte(healthyYAML)), strings.Repeat("a", 40)
		p, err := NewPlan(s, digest, commit, 1, time.Now(), refs())
		if err != nil {
			t.Fatal(err)
		}
		mutate(p)
		if err := p.Validate(s, digest, commit); err == nil {
			t.Fatal("accepted altered plan")
		}
	}
}

func TestIdentityTagsCannotBeOverridden(t *testing.T) {
	for _, tag := range []string{"bssid:02:00:00:00:00:01", "SSID:known", "hostname:known", "infra_mode:host", "network_id:known", "dd.internal.resource:known", "device_namespace:known"} {
		s := healthy(t)
		s.Fleet[0].Tags = []string{tag}
		if err := s.Validate(); err == nil {
			t.Errorf("accepted identity override %q", tag)
		}
	}
}

func TestProcessEvidenceRequiresReconciliationMetrics(t *testing.T) {
	s := healthy(t)
	s.Phases[0].Processes = map[string][]ProcessDef{"mac": {{Name: "Chrome", CPU: Pattern{Steady: &SteadyPattern{Value: 2}}, Memory: Pattern{Steady: &SteadyPattern{Value: 100}}}}}
	profile := refs()["mac"].Profile
	if err := s.ValidateEvidence(s.Fleet[0], profile); err == nil {
		t.Fatal("accepted process overlay without host resource metrics")
	}
	profile.MetricNames = []string{"system.cpu.user", "system.cpu.system", "system.cpu.idle", "system.mem.used", "system.mem.free", "system.mem.usable", "system.mem.pct_usable"}
	if err := s.ValidateEvidence(s.Fleet[0], profile); err != nil {
		t.Fatal(err)
	}
	s.Phases[0].Metrics = map[string]map[string]Pattern{"mac": {"system.cpu.user": {Steady: &SteadyPattern{Value: 5}}}}
	if err := s.ValidateEvidence(s.Fleet[0], profile); err == nil {
		t.Fatal("accepted conflicting host/process resource overlays")
	}
}
