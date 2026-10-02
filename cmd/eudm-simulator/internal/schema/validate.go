// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// knownMetrics is the set of metric names valid in the YAML metrics block.
// Any other name is likely a typo since the simulator only knows how to send these.
var knownMetrics = map[string]bool{
	"system.cpu.user":                     true,
	"system.mem.pct_usable":               true,
	"system.disk.utilized":                true,
	"system.uptime":                       true,
	"system.battery.maximum_capacity_pct": true,
	"system.battery.current_charge_pct":   true,
	"system.battery.cycle_count":          true,
	"system.battery.charge_rate":          true,
	"system.wlan.rssi":                    true,
	"system.wlan.noise":                   true,
	"system.wlan.txrate":                  true,
	"system.wlan.rxrate":                  true,
	"system.net.packets_in.drop":          true,
	"system.net.packets_out.drop":         true,
	"system.net.packets_in.error":         true,
	"system.net.packets_out.error":        true,
	"system.net.tcp.retrans_segs":         true,
}

// metricBounds defines [min, max] expected ranges for metrics where out-of-range
// values are clearly wrong (percentages, fractions, dBm readings).
var metricBounds = map[string][2]float64{
	"system.cpu.user":                     {0, 100},
	"system.mem.pct_usable":               {0, 1},
	"system.disk.utilized":                {0, 100},
	"system.battery.maximum_capacity_pct": {0, 100},
	"system.battery.current_charge_pct":   {0, 100},
	"system.wlan.rssi":                    {-120, 0},
	"system.wlan.noise":                   {-120, 0},
	"system.uptime":                       {0, 1e9},
	"system.wlan.txrate":                  {0, 10000},
	"system.wlan.rxrate":                  {0, 10000},
}

// validSoftwareTypes lists valid software_type values per OS.
var validSoftwareTypes = map[string][]string{
	"macos":   {"app", "homebrew", "pkg"},
	"windows": {"desktop", "msstore", "msi"},
}

// validDeploymentStatuses is the set of accepted deployment_status values.
var validDeploymentStatuses = map[string]bool{
	"installed":       true,
	"absent":          true,
	"pending_install": true,
	"pending_removal": true,
}

// Validate checks the scenario for correctness and returns an error describing
// all problems found. The error lists every issue so the author can fix them all
// in one pass rather than iterating one error at a time.
func (s *Scenario) Validate() error {
	var errs []string
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// ── scenario meta ──────────────────────────────────────────────────────────
	if strings.TrimSpace(s.Meta.Name) == "" {
		add("scenario.name: required")
	}

	// ── fleet ──────────────────────────────────────────────────────────────────
	if len(s.Fleet) == 0 {
		add("fleet: must define at least one group")
	}

	groupNames := map[string]bool{}
	groupOS := map[string]string{}
	for i, g := range s.Fleet {
		pfx := fmt.Sprintf("fleet[%d]", i)

		if strings.TrimSpace(g.Group) == "" {
			add("%s.group: required", pfx)
		} else {
			if groupNames[g.Group] {
				add("%s.group: %q is already defined earlier in the fleet; group names must be unique", pfx, g.Group)
			}
			groupNames[g.Group] = true
			groupOS[g.Group] = g.OS
			if containsWhitespace(g.Group) {
				add("%s.group: %q must not contain whitespace", pfx, g.Group)
			}
		}

		if g.Count <= 0 {
			add("%s.count: must be > 0, got %d", pfx, g.Count)
		} else if g.Count > 10000 {
			add("%s.count: %d exceeds maximum of 10000", pfx, g.Count)
		}

		switch g.OS {
		case "macos", "windows":
		case "":
			add("%s.os: required — must be one of: macos, windows", pfx)
		default:
			add("%s.os: %q is not valid — must be one of: macos, windows", pfx, g.OS)
		}

		if math.IsNaN(g.BaselineVariance) || math.IsInf(g.BaselineVariance, 0) || g.BaselineVariance < 0 || g.BaselineVariance > 1 {
			add("%s.baseline_variance: must be in [0.0, 1.0], got %g", pfx, g.BaselineVariance)
		}

		for j, tag := range g.Tags {
			if !strings.Contains(tag, ":") {
				add("%s.tags[%d]: %q is missing a colon separator — expected format is key:value", pfx, j, tag)
			} else if strings.HasPrefix(tag, ":") || strings.HasSuffix(tag, ":") {
				add("%s.tags[%d]: %q has an empty key or value — expected format is key:value", pfx, j, tag)
			}
		}
	}

	// ── software_inventory ─────────────────────────────────────────────────────
	for group, items := range s.Software {
		pfx := fmt.Sprintf("software_inventory[%s]", group)
		if !groupNames[group] {
			add("%s: %q does not match any fleet group name", pfx, group)
			continue
		}
		os := groupOS[group]
		seenNames := map[string]bool{}
		for i, item := range items {
			ipfx := fmt.Sprintf("%s[%d]", pfx, i)

			if strings.TrimSpace(item.Name) == "" {
				add("%s.name: required", ipfx)
			} else if seenNames[item.Name] {
				add("%s.name: %q appears more than once in this group's software list", ipfx, item.Name)
			} else {
				seenNames[item.Name] = true
			}

			if strings.TrimSpace(item.Version) == "" {
				add("%s.version: required", ipfx)
			}

			if item.SoftwareType != "" {
				valid := validSoftwareTypes[os]
				if !containsString(valid, item.SoftwareType) {
					add("%s.software_type: %q is not valid for %s — must be one of: %s",
						ipfx, item.SoftwareType, os, strings.Join(valid, ", "))
				}
			}

			if item.DeploymentStatus != "" && !validDeploymentStatuses[item.DeploymentStatus] {
				add("%s.deployment_status: %q is not valid — must be one of: installed, absent, pending_install, pending_removal",
					ipfx, item.DeploymentStatus)
			}

			if item.DeploymentTime != "" {
				if _, err := time.Parse(time.RFC3339, item.DeploymentTime); err != nil {
					add("%s.deployment_time: %q is not valid RFC3339 (e.g. 2024-01-15T09:00:00Z)",
						ipfx, item.DeploymentTime)
				}
			}

			if os == "windows" && item.ProductCode != "" {
				if msg := validateProductCode(item.ProductCode, ipfx+".product_code"); msg != "" {
					errs = append(errs, msg)
				}
			}
		}
	}

	// ── phases ─────────────────────────────────────────────────────────────────
	if len(s.Phases) == 0 {
		add("phases: must define at least one phase")
	}

	phaseNames := map[string]bool{}
	for i, ph := range s.Phases {
		pfx := fmt.Sprintf("phases[%d]", i)
		if ph.Name != "" {
			pfx = fmt.Sprintf("phases[%d] (%q)", i, ph.Name)
		}

		if strings.TrimSpace(ph.Name) == "" {
			add("%s.name: required", pfx)
		} else if phaseNames[ph.Name] {
			add("%s.name: duplicate phase name %q", pfx, ph.Name)
		} else {
			phaseNames[ph.Name] = true
		}

		if ph.Duration.Duration <= 0 {
			add("%s.duration: required and must be > 0 (e.g. \"5m\", \"30s\", \"1h\")", pfx)
		}

		if math.IsNaN(ph.JitterScale) || math.IsInf(ph.JitterScale, 0) || ph.JitterScale < 0 {
			add("%s.jitter_scale: must be >= 0, got %g", pfx, ph.JitterScale)
		}

		for gname := range ph.Processes {
			if !groupNames[gname] {
				add("%s.processes: group %q does not exist in fleet — valid groups: %s",
					pfx, gname, joinedGroupNames(groupNames))
			}
		}
		for gname := range ph.Metrics {
			if !groupNames[gname] {
				add("%s.metrics: group %q does not exist in fleet — valid groups: %s",
					pfx, gname, joinedGroupNames(groupNames))
			}
		}
		// ── processes ────────────────────────────────────────────────────────
		for gname, procs := range ph.Processes {
			procNames := map[string]bool{}
			var cpuTotal float64
			for j, proc := range procs {
				ppfx := fmt.Sprintf("%s.processes[%s][%d]", pfx, gname, j)

				if strings.TrimSpace(proc.Name) == "" {
					add("%s.name: required", ppfx)
				} else {
					if procNames[proc.Name] {
						add("%s.name: %q appears more than once in group %q for this phase",
							ppfx, proc.Name, gname)
					}
					procNames[proc.Name] = true
				}

				if isZeroPattern(proc.CPU) {
					add("%s.cpu: required — set one of: steady, ramp, spike, step", ppfx)
				} else if err := validateProcessPattern(proc.CPU, ppfx+".cpu"); err != nil {
					add("%s", err)
				}

				if isZeroPattern(proc.Memory) {
					add("%s.memory: required — set one of: steady, ramp, spike, step", ppfx)
				} else if err := validateProcessPattern(proc.Memory, ppfx+".memory"); err != nil {
					add("%s", err)
				}

				cpuTotal += peakCPU(proc.CPU)
			}

			if cpuTotal > 100 {
				add("%s.processes[%s]: combined peak CPU is %.1f%% which exceeds the 100%% whole-host resource limit",
					pfx, gname, cpuTotal)
			}
		}

		// ── metrics ──────────────────────────────────────────────────────────
		for gname, metrics := range ph.Metrics {
			for mname, pat := range metrics {
				mpfx := fmt.Sprintf("%s.metrics[%s].%s", pfx, gname, mname)

				if !knownMetrics[mname] {
					add("%s: unknown metric name — known metrics: %s",
						mpfx, joinedSortedKeys(knownMetrics))
				}

				if isZeroPattern(pat) {
					add("%s: pattern is empty — set one of: steady, ramp, spike, step", mpfx)
					continue
				}
				if err := validatePattern(pat, mpfx); err != nil {
					add("%s", err)
				}
				if bounds, ok := metricBounds[mname]; ok {
					errs = append(errs, patternBoundsErrors(pat, mpfx, bounds[0], bounds[1])...)
				}
			}
		}

	}

	// ── network_devices (access points) ─────────────────────────────────────────
	validateNetworkDevices(s, add)
	validateContracts(s, add)

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("scenario validation failed (%d error(s)):\n  - %s",
		len(errs), strings.Join(errs, "\n  - "))
}

// ── pattern validation ─────────────────────────────────────────────────────────

// validatePattern checks structural correctness of a pattern: exactly one type
// set, and type-specific constraints on at/duration. It does NOT enforce
// non-negativity — callers that require non-negative values (e.g. process cpu/
// memory) should call validateProcessPattern instead.
func validatePattern(p Pattern, field string) error {
	if errs := patternBoundsErrors(p, field, -math.MaxFloat64, math.MaxFloat64); len(errs) > 0 {
		return fmt.Errorf("%s", errs[0])
	}
	set := 0
	if p.Steady != nil {
		set++
	}
	if p.Ramp != nil {
		set++
	}
	if p.Spike != nil {
		set++
		if math.IsNaN(p.Spike.At) || p.Spike.At < 0 || p.Spike.At > 100 {
			return fmt.Errorf("%s: spike.at must be 0–100, got %g", field, p.Spike.At)
		}
		if math.IsNaN(p.Spike.Duration) || p.Spike.Duration <= 0 || p.Spike.Duration > 100 {
			return fmt.Errorf("%s: spike.duration must be > 0 and <= 100, got %g", field, p.Spike.Duration)
		}
	}
	if p.Step != nil {
		set++
		if math.IsNaN(p.Step.At) || p.Step.At < 0 || p.Step.At > 100 {
			return fmt.Errorf("%s: step.at must be 0–100, got %g", field, p.Step.At)
		}
	}
	if set == 0 {
		return fmt.Errorf("%s: no pattern type set — use one of: steady, ramp, spike, step", field)
	}
	if set > 1 {
		return fmt.Errorf("%s: only one pattern type may be set at a time (%d are set)", field, set)
	}
	return nil
}

// validateProcessPattern extends validatePattern with non-negativity checks,
// appropriate for process cpu and memory fields which cannot be negative.
func validateProcessPattern(p Pattern, field string) error {
	if err := validatePattern(p, field); err != nil {
		return err
	}
	switch {
	case p.Steady != nil && p.Steady.Value < 0:
		return fmt.Errorf("%s: steady.value must be >= 0, got %g", field, p.Steady.Value)
	case p.Ramp != nil && p.Ramp.From < 0:
		return fmt.Errorf("%s: ramp.from must be >= 0, got %g", field, p.Ramp.From)
	case p.Ramp != nil && p.Ramp.To < 0:
		return fmt.Errorf("%s: ramp.to must be >= 0, got %g", field, p.Ramp.To)
	case p.Spike != nil && p.Spike.Baseline < 0:
		return fmt.Errorf("%s: spike.baseline must be >= 0, got %g", field, p.Spike.Baseline)
	case p.Spike != nil && p.Spike.Peak < 0:
		return fmt.Errorf("%s: spike.peak must be >= 0, got %g", field, p.Spike.Peak)
	case p.Step != nil && p.Step.Before < 0:
		return fmt.Errorf("%s: step.before must be >= 0, got %g", field, p.Step.Before)
	case p.Step != nil && p.Step.After < 0:
		return fmt.Errorf("%s: step.after must be >= 0, got %g", field, p.Step.After)
	}
	return nil
}

// patternBoundsErrors returns an error string for each pattern value outside [lo, hi].
func patternBoundsErrors(p Pattern, field string, lo, hi float64) []string {
	var out []string
	check := func(v float64, sub string) {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < lo || v > hi {
			out = append(out, fmt.Sprintf("%s: %s value %g is outside expected range [%g, %g]",
				field, sub, v, lo, hi))
		}
	}
	if p.Steady != nil {
		check(p.Steady.Value, "steady")
	}
	if p.Ramp != nil {
		check(p.Ramp.From, "ramp.from")
		check(p.Ramp.To, "ramp.to")
	}
	if p.Spike != nil {
		check(p.Spike.Baseline, "spike.baseline")
		check(p.Spike.Peak, "spike.peak")
	}
	if p.Step != nil {
		check(p.Step.Before, "step.before")
		check(p.Step.After, "step.after")
	}
	return out
}

// isZeroPattern reports whether no pattern type has been set.
func isZeroPattern(p Pattern) bool {
	return p.Steady == nil && p.Ramp == nil && p.Spike == nil && p.Step == nil
}

// peakCPU returns the maximum CPU value reachable by a pattern.
// Used to estimate worst-case total CPU across all processes in a group.
func peakCPU(p Pattern) float64 {
	switch {
	case p.Steady != nil:
		return p.Steady.Value
	case p.Ramp != nil:
		if p.Ramp.From > p.Ramp.To {
			return p.Ramp.From
		}
		return p.Ramp.To
	case p.Spike != nil:
		return p.Spike.Peak
	case p.Step != nil:
		if p.Step.Before > p.Step.After {
			return p.Step.Before
		}
		return p.Step.After
	}
	return 0
}

// ── software validation helpers ───────────────────────────────────────────────

func validateProductCode(code, field string) string {
	// Windows product codes are GUIDs: {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}
	if len(code) == 38 && code[0] == '{' && code[37] == '}' {
		return ""
	}
	return fmt.Sprintf("%s: %q does not look like a Windows GUID (expected {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX})", field, code)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func containsWhitespace(s string) bool {
	return strings.ContainsAny(s, " \t\n\r")
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func joinedGroupNames(groups map[string]bool) string {
	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func joinedSortedKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
