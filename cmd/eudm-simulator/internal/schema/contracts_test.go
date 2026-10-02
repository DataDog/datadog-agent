// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"math"
	"reflect"
	"slices"
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

func baselineRef(platform string) BundleRef {
	return BundleRef{Digest: Digest([]byte(platform)), CaptureToolCommit: strings.Repeat("a", 40), Profile: Profile{OS: platform, Architecture: "arm64", Streams: []Stream{Metrics, HostMetadata, AgentInventory, HostInventory, Processes, Software}, MetricNames: []string{"system.cpu.user"}, ProcessNames: []string{"Chrome"}, SoftwareNames: []string{"Google Chrome"}}}
}

func TestSingleBaselinePlanRoundTrip(t *testing.T) {
	s := healthy(t)
	s.Fleet[1].OS = "macos"
	digest, commit := Digest([]byte(healthyYAML)), strings.Repeat("a", 40)
	baseline := baselineRef("macos")
	p, err := NewPlan(s, digest, commit, 42, time.Date(2026, 9, 29, 1, 2, 3, 0, time.UTC), baseline)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != RunPlanVersion || !reflect.DeepEqual(p.Bundle, baseline) {
		t.Fatal("plan did not retain the single baseline capture", p)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"bundles"`) || strings.Contains(string(data), `"assignments"`) {
		t.Fatal("plan retained the removed cohort assignment contract")
	}
	var contract struct {
		Bundle map[string]json.RawMessage `json:"bundle"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Bundle["capture_tool_commit"] == nil || contract.Bundle["agent_commit"] != nil {
		t.Fatal("bundle revision must identify the capture tool, independently of producers")
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
		mutate func(*Scenario, *BundleRef)
	}{
		{"missing baseline", func(_ *Scenario, r *BundleRef) { *r = BundleRef{} }},
		{"OS mismatch", func(_ *Scenario, r *BundleRef) { r.Profile.OS = "windows" }},
		{"incompatible second group", func(s *Scenario, _ *BundleRef) { s.Fleet[1].OS = "windows" }},
		{"missing stream", func(_ *Scenario, r *BundleRef) { r.Profile.Streams = []Stream{Metrics} }},
		{"wrong capture tool commit", func(_ *Scenario, r *BundleRef) { r.CaptureToolCommit = strings.Repeat("b", 40) }},
		{"invented hardware", func(s *Scenario, _ *BundleRef) { s.Fleet[0].TotalRAMGB = 128 }},
		{"invented metric", func(s *Scenario, _ *BundleRef) {
			s.Phases[0].Metrics = map[string]map[string]Pattern{"mac": {"system.wlan.rssi": {Steady: &SteadyPattern{Value: -55}}}}
		}},
		{"missing metric for second group", func(s *Scenario, _ *BundleRef) {
			s.Phases[0].Metrics = map[string]map[string]Pattern{"win": {"system.wlan.rssi": {Steady: &SteadyPattern{Value: -55}}}}
		}},
		{"missing connections for second group", func(s *Scenario, _ *BundleRef) {
			s.Phases[0].Connections = map[string][]ConnectionOverlay{"win": {{Selector: "vpn-1", RTTMilliseconds: &Pattern{Steady: &SteadyPattern{Value: 400}}}}}
		}},
		{"invented process", func(s *Scenario, _ *BundleRef) {
			s.Phases[0].Processes = map[string][]ProcessDef{"win": {{Name: "not-captured", CPU: Pattern{Steady: &SteadyPattern{Value: 1}}, Memory: Pattern{Steady: &SteadyPattern{Value: 20}}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := healthy(t), baselineRef("macos")
			s.Fleet[1].OS = "macos"
			tc.mutate(s, &r)
			if _, err := NewPlan(s, Digest([]byte(healthyYAML)), strings.Repeat("a", 40), 1, time.Now(), r); err == nil {
				t.Fatal("accepted invalid evidence")
			}
		})
	}
}

func TestPlanRequiresBothDeviceRegistrationInventories(t *testing.T) {
	for _, stream := range []Stream{AgentInventory, HostInventory} {
		s, ref := healthy(t), baselineRef("macos")
		s.Fleet[1].OS = "macos"
		ref.Profile.Streams = slices.DeleteFunc(ref.Profile.Streams, func(s Stream) bool { return s == stream })
		if _, err := NewPlan(s, Digest([]byte(healthyYAML)), strings.Repeat("a", 40), 1, time.Now(), ref); err == nil || !strings.Contains(err.Error(), string(stream)) {
			t.Fatalf("accepted baseline without %s: %v", stream, err)
		}
	}
	s, ref := healthy(t), baselineRef("macos")
	s.Fleet[1].OS = "macos"
	p, err := NewPlan(s, Digest([]byte(healthyYAML)), strings.Repeat("a", 40), 1, time.Now(), ref)
	if err != nil {
		t.Fatal(err)
	}
	p.Version = 3
	if err := p.Validate(s, Digest([]byte(healthyYAML)), strings.Repeat("a", 40)); err == nil {
		t.Fatal("accepted old plan lacking required inventory contract")
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
	b := baselineRef("windows")
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
		func(p *RunPlan) { p.Version = 1 }, func(p *RunPlan) { p.Version = 2 }, func(p *RunPlan) { p.Bundle = BundleRef{} },
		func(p *RunPlan) { p.Bundle.CaptureToolCommit = strings.Repeat("b", 40) },
		func(p *RunPlan) { p.Bundle.Digest = "invalid" },
		func(p *RunPlan) { p.Bundle.Profile.Streams = nil },
	} {
		s := healthy(t)
		s.Fleet[1].OS = "macos"
		digest, commit := Digest([]byte(healthyYAML)), strings.Repeat("a", 40)
		p, err := NewPlan(s, digest, commit, 1, time.Now(), baselineRef("macos"))
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
	profile := baselineRef("macos").Profile
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
