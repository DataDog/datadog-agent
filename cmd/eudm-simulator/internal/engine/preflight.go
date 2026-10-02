// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"fmt"
	"math"
	"slices"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/overlay"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// validateOverlays checks conservative bounds, including the full variation
// envelope, against every captured cycle before delivery starts. CPU and memory
// reconciliation are monotone in their target values, so checking both bounds
// also covers every device and intermediate point without expanding the fleet.
func validateOverlays(s *schema.Scenario, group schema.GroupDef, b *bundle.Loaded, timelines map[schema.Stream][]*timeline) error {
	var baselines [][]*model.CollectorProc
	for _, cycle := range timelines[schema.Processes][0].cycles {
		var chunks []*model.CollectorProc
		for _, ref := range cycle.refs {
			sample, err := decodeCapturedSample(b, ref)
			if err != nil {
				return err
			}
			chunks = append(chunks, sample.Processes)
		}
		baselines = append(baselines, chunks)
	}
	for phaseIndex, phase := range s.Phases {
		for _, name := range requiredMetricEvidence(phase, group) {
			family := telemetrycapture.MetricFamily(name)
			if !slices.ContainsFunc(timelines[schema.Metrics], func(t *timeline) bool { return t.family == family }) {
				return fmt.Errorf("cohort %q phase %q lacks captured metric family for %q", group.Group, phase.Name, name)
			}
		}
		for _, upper := range []bool{false, true} {
			bounded := *s
			bounded.Phases = slices.Clone(s.Phases)
			bounded.Phases[phaseIndex] = boundedPhase(phase, group, upper)
			fixedGroup := group
			fixedGroup.BaselineVariance = 0
			for _, stream := range streamOrder {
				for _, timeline := range timelines[stream] {
					for cycleIndex, cycle := range timeline.cycles {
						if err := validateCycleEvidence(b, cycle, stream, timeline.family, phase, group); err != nil {
							return err
						}
						selected := baselines[:1]
						if stream == schema.Processes {
							selected = baselines[cycleIndex : cycleIndex+1]
						}
						if stream == schema.Metrics && len(phase.Processes[group.Group]) > 0 {
							selected = baselines
						}
						for _, baseline := range selected {
							ctx := overlay.Context{Scenario: &bounded, Group: fixedGroup, PhaseIndex: phaseIndex, Stream: stream, BaselineProcesses: baseline}
							for _, ref := range cycle.refs {
								sample, err := decodeTimelineSample(b, ref, timeline.family)
								if err == nil {
									err = overlay.Apply(ctx, sample)
								}
								if err != nil {
									return fmt.Errorf("cohort %q phase %q %s preflight: %w", group.Group, phase.Name, stream, err)
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// Profile inventories are unions; each replay collection must itself contain
// the evidence targeted within its check family, including multi-chunk groups.
func validateCycleEvidence(b *bundle.Loaded, cycle cycle, stream schema.Stream, family string, phase schema.Phase, group schema.GroupDef) error {
	names := map[string]bool{}
	for _, ref := range cycle.refs {
		sample, err := decodeTimelineSample(b, ref, family)
		if err != nil {
			return err
		}
		for _, metric := range sample.Metrics {
			names[metric.Name] = true
			if (group.AccessPoint != "" || group.BSSID != "" || group.SSID != "") && slices.Contains([]string{"system.wlan.rssi", "system.wlan.noise", "system.wlan.txrate", "system.wlan.rxrate"}, metric.Name) {
				tags := map[string]bool{}
				for _, tag := range metric.Tags.UnsafeToReadOnlySliceString() {
					key, value, ok := strings.Cut(tag, ":")
					if ok && value != "" {
						tags[key] = true
					}
				}
				if !tags["bssid"] || !tags["ssid"] || (!tags["client_mac"] && !tags["mac_address"]) {
					return fmt.Errorf("cohort %q: captured WLAN series lacks wireless identity tags", group.Group)
				}
			}
		}
		if sample.Connections != nil {
			for _, connection := range sample.Connections.Connections {
				names[telemetry.ConnectionSelector(connection)] = true
			}
		}
	}
	var required []string
	if stream == schema.Metrics {
		for _, name := range requiredMetricEvidence(phase, group) {
			if telemetrycapture.MetricFamily(name) == family {
				required = append(required, name)
			}
		}
	}
	if stream == schema.Connections {
		for _, connection := range phase.Connections[group.Group] {
			required = append(required, connection.Selector)
		}
	}
	for _, name := range required {
		if !names[name] {
			return fmt.Errorf("cohort %q phase %q: a captured %s cycle lacks required %q", group.Group, phase.Name, stream, name)
		}
	}
	return nil
}

func requiredMetricEvidence(phase schema.Phase, group schema.GroupDef) []string {
	var required []string
	for name := range phase.Metrics[group.Group] {
		required = append(required, name)
	}
	if len(phase.Processes[group.Group]) > 0 {
		required = append(required, "system.cpu.user", "system.cpu.system", "system.cpu.idle", "system.mem.used", "system.mem.free", "system.mem.usable", "system.mem.pct_usable")
	}
	if group.AccessPoint != "" || group.BSSID != "" || group.SSID != "" {
		required = append(required, "system.wlan.rssi", "system.wlan.noise", "system.wlan.txrate", "system.wlan.rxrate")
	}
	return required
}

func boundedPhase(phase schema.Phase, group schema.GroupDef, upper bool) schema.Phase {
	spread := min(1.0, group.BaselineVariance*phase.EffectiveJitterScale())
	bound := func(p schema.Pattern) schema.Pattern { return boundPattern(p, spread, upper) }
	processes := slices.Clone(phase.Processes[group.Group])
	for i := range processes {
		processes[i].CPU, processes[i].Memory = bound(processes[i].CPU), bound(processes[i].Memory)
	}
	phase.Processes = map[string][]schema.ProcessDef{group.Group: processes}
	metrics := map[string]schema.Pattern{}
	for name, pattern := range phase.Metrics[group.Group] {
		metrics[name] = bound(pattern)
	}
	phase.Metrics = map[string]map[string]schema.Pattern{group.Group: metrics}
	connections := slices.Clone(phase.Connections[group.Group])
	for i := range connections {
		for _, field := range []**schema.Pattern{&connections[i].RTTMilliseconds, &connections[i].RTTVarianceMilliseconds, &connections[i].Retransmits} {
			if *field != nil {
				value := bound(**field)
				*field = &value
			}
		}
		failures := map[uint32]schema.Pattern{}
		for code, pattern := range connections[i].TCPFailures {
			failures[code] = bound(pattern)
		}
		connections[i].TCPFailures = failures
	}
	phase.Connections = map[string][]schema.ConnectionOverlay{group.Group: connections}
	return phase
}

func boundPattern(pattern schema.Pattern, spread float64, upper bool) schema.Pattern {
	var values []float64
	if p := pattern.Steady; p != nil {
		values = append(values, p.Value)
	}
	if p := pattern.Ramp; p != nil {
		values = append(values, p.From, p.To)
	}
	if p := pattern.Step; p != nil {
		values = append(values, p.Before, p.After)
	}
	if p := pattern.Spike; p != nil {
		values = append(values, p.Baseline, p.Peak)
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, value := range values {
		lo = min(lo, value*(1-spread), value*(1+spread))
		hi = max(hi, value*(1-spread), value*(1+spread))
	}
	if upper {
		lo = hi
	}
	return schema.Pattern{Steady: &schema.SteadyPattern{Value: lo}}
}
