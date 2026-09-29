// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Version identifies the scenario and run-plan contracts.
const Version = 1

// Conclusion is a local acceptance criterion, never a telemetry dimension.
type Conclusion string

const (
	Healthy                Conclusion = "healthy"
	ProcessSoftwareVersion Conclusion = "process_software_version"
	VPNPath                Conclusion = "vpn_path"
	WirelessAccessPoints   Conclusion = "wireless_access_points"
)

// Expectation is written only to the local report.
type Expectation struct {
	AffectedCohorts []string   `yaml:"affected_cohorts" json:"affected_cohorts"`
	Conclusion      Conclusion `yaml:"conclusion" json:"conclusion"`
}

// Stream identifies a captured typed output boundary.
type Stream string

const (
	Metrics      Stream = "metrics"
	HostMetadata Stream = "host_metadata"
	Processes    Stream = "processes"
	Connections  Stream = "connections"
	Software     Stream = "software"
)

// ConnectionOverlay selects only records with the given sanitized capture ID.
// The bundle, rather than a host OS guess, determines whether evidence exists.
type ConnectionOverlay struct {
	Selector                string             `yaml:"selector"`
	RTTMilliseconds         *Pattern           `yaml:"rtt_ms"`
	RTTVarianceMilliseconds *Pattern           `yaml:"rtt_variance_ms"`
	Retransmits             *Pattern           `yaml:"retransmits"`
	TCPFailures             map[uint32]Pattern `yaml:"tcp_failures"`
}

// RequiredStreams returns streams that must exist before any device is sent.
func (s *Scenario) RequiredStreams(cohort string) []Stream {
	streams := []Stream{Metrics, HostMetadata, Processes, Software}
	for _, phase := range s.Phases {
		if len(phase.Connections[cohort]) > 0 {
			return append(streams, Connections)
		}
	}
	return streams
}

func validateContracts(s *Scenario, add func(string, ...any)) {
	if s.Version != Version {
		add("version: expected %d", Version)
	}
	var total int
	for _, group := range s.Fleet {
		if group.Count > 0 && total > math.MaxInt-group.Count {
			add("fleet: device count overflows")
			break
		}
		total += group.Count
		if group.TotalRAMGB < 0 {
			add("fleet[%s].total_ram_gb: cannot be negative", group.Group)
		}
		for _, tag := range group.Tags {
			validateTag(tag, s.Meta.Name, add)
		}
	}
	seen := map[string]bool{}
	for _, group := range s.Expectation.AffectedCohorts {
		if s.GroupByName(group) == nil || seen[group] {
			add("expectation.affected_cohorts: unknown or repeated cohort %q", group)
		}
		seen[group] = true
	}
	switch s.Expectation.Conclusion {
	case Healthy:
		if len(seen) != 0 {
			add("expectation: healthy requires no affected cohorts")
		}
	case ProcessSoftwareVersion, VPNPath, WirelessAccessPoints:
		if len(seen) == 0 {
			add("expectation: incident requires affected cohorts")
		}
		if s.MonitorWindow.Duration <= 0 || s.VisibilityDelay.Duration < 0 {
			add("monitor_window must be positive and visibility_delay cannot be negative")
		}
		want := []string{"healthy", "onset", "sustained", "recovery"}
		if len(s.Phases) != len(want) {
			add("phases: incidents require healthy, onset, sustained, recovery")
		} else {
			for i, phase := range s.Phases {
				if phase.Name != want[i] {
					add("phases[%d]: expected %q", i, want[i])
				}
				if (i == 0 || i == 2) && (phase.Duration.Duration < s.MonitorWindow.Duration || phase.Duration.Duration-s.MonitorWindow.Duration < s.VisibilityDelay.Duration) {
					add("phases[%d]: must cover monitor_window plus visibility_delay", i)
				}
			}
		}
	default:
		add("expectation.conclusion: unsupported value %q", s.Expectation.Conclusion)
	}
	var duration time.Duration
	for _, phase := range s.Phases {
		if phase.Duration.Duration > 0 && duration > time.Duration(math.MaxInt64)-phase.Duration.Duration {
			add("phases: duration overflows")
			break
		}
		duration += phase.Duration.Duration
		for group, overlays := range phase.Connections {
			if s.GroupByName(group) == nil {
				add("connections: unknown cohort %q", group)
			}
			selectors := map[string]bool{}
			for _, overlay := range overlays {
				if strings.TrimSpace(overlay.Selector) == "" || selectors[overlay.Selector] {
					add("connections[%s]: missing or duplicate captured selector", group)
				}
				selectors[overlay.Selector] = true
				if overlay.RTTMilliseconds == nil && overlay.RTTVarianceMilliseconds == nil && overlay.Retransmits == nil && len(overlay.TCPFailures) == 0 {
					add("connections[%s]: overlay must change a field", group)
				}
				for _, p := range []*Pattern{overlay.RTTMilliseconds, overlay.RTTVarianceMilliseconds, overlay.Retransmits} {
					if p != nil {
						if err := validateProcessPattern(*p, "connections["+group+"]"); err != nil {
							add("%s", err)
						}
					}
				}
				for code, pattern := range overlay.TCPFailures {
					// Agent connection payloads use POSIX error numbers on Windows too.
					if !slices.Contains([]uint32{104, 110, 111, 125}, code) {
						add("tcp_failures: unsupported standardized code %d", code)
					}
					if err := validateProcessPattern(pattern, "tcp_failures"); err != nil {
						add("%s", err)
					}
					for _, err := range patternBoundsErrors(pattern, "tcp_failures", 0, math.MaxUint32) {
						add("%s", err)
					}
				}
			}
		}
		for group, software := range phase.Software {
			if s.GroupByName(group) == nil {
				add("software_inventory: unknown cohort %q", group)
			}
			names := map[string]bool{}
			for _, item := range software {
				if item.Name == "" || item.Version == "" || names[item.Name] {
					add("software_inventory[%s]: name and version required without duplicate applications", group)
				}
				names[item.Name] = true
			}
		}
	}
	for _, ap := range s.NetworkDevices.AccessPoints {
		for _, tag := range ap.Tags {
			validateTag(tag, s.Meta.Name, add)
		}
	}
}

func validateTag(tag, scenario string, add func(string, ...any)) {
	key, value, ok := strings.Cut(tag, ":")
	if !ok || key == "" || value == "" {
		add("tag %q: expected key:value", tag)
		return
	}
	switch strings.ToLower(key) {
	case "scenario", "scenario_name", "affected", "affected_status", "cohort", "expected", "expectation", "conclusion", "run_id", "eudm_run_id", "host", "host_id", "hostname", "host_uuid", "network_id", "network-id", "bssid", "ssid", "client_mac", "mac_address", "infra_mode", "infrastructure_mode", "device_id", "device_namespace", "device_ip", "snmp_device", "device_hostname", "dd.internal.resource":
		add("tag key %q is reserved or discloses a local expectation", key)
	}
	if scenario != "" && strings.Contains(strings.ToLower(value), strings.ToLower(scenario)) {
		add("tag %q discloses scenario identity", tag)
	}
}

// ValidateEvidence proves that overlays refer to captured evidence; no profile
// is invented to satisfy an otherwise valid declaration.
func (s *Scenario) ValidateEvidence(group GroupDef, profile Profile) error {
	if profile.OS != group.OS {
		return fmt.Errorf("cohort %q requires %s bundle, got %s", group.Group, group.OS, profile.OS)
	}
	if group.TotalRAMGB != 0 && profile.MemoryBytes != uint64(group.TotalRAMGB)*1024*1024*1024 {
		return fmt.Errorf("cohort %q requires a capture with its declared RAM", group.Group)
	}
	for _, stream := range s.RequiredStreams(group.Group) {
		if !slices.Contains(profile.Streams, stream) {
			return fmt.Errorf("cohort %q: bundle lacks required stream %s; capture a compatible bundle separately", group.Group, stream)
		}
	}
	checkSoftware := func(items []SoftwareItem) error {
		for _, item := range items {
			if !slices.Contains(profile.SoftwareNames, item.Name) {
				return fmt.Errorf("cohort %q: application %q was not captured", group.Group, item.Name)
			}
		}
		return nil
	}
	if group.AccessPoint != "" || group.BSSID != "" || group.SSID != "" {
		for _, name := range []string{"system.wlan.rssi", "system.wlan.noise", "system.wlan.txrate", "system.wlan.rxrate"} {
			if !slices.Contains(profile.MetricNames, name) {
				return fmt.Errorf("cohort %q: wireless association requires captured %s", group.Group, name)
			}
		}
	}
	if err := checkSoftware(s.Software[group.Group]); err != nil {
		return err
	}
	for _, phase := range s.Phases {
		if len(phase.Processes[group.Group]) > 0 {
			for _, metric := range []string{"system.cpu.user", "system.cpu.system", "system.cpu.idle", "system.mem.used", "system.mem.free", "system.mem.usable", "system.mem.pct_usable"} {
				if _, explicit := phase.Metrics[group.Group][metric]; explicit {
					return fmt.Errorf("cohort %q: %s conflicts with process resource reconciliation", group.Group, metric)
				}
				if !slices.Contains(profile.MetricNames, metric) {
					return fmt.Errorf("cohort %q: process overlays require captured %s for resource reconciliation", group.Group, metric)
				}
			}
		}
		for _, proc := range phase.Processes[group.Group] {
			if !slices.Contains(profile.ProcessNames, proc.Name) {
				return fmt.Errorf("cohort %q: process %q was not captured", group.Group, proc.Name)
			}
		}
		for metric := range phase.Metrics[group.Group] {
			if !slices.Contains(profile.MetricNames, metric) {
				return fmt.Errorf("cohort %q: metric %q was not captured", group.Group, metric)
			}
		}
		for _, overlay := range phase.Connections[group.Group] {
			if !slices.Contains(profile.ConnectionSelectors, overlay.Selector) {
				return fmt.Errorf("cohort %q: connection selector %q was not captured", group.Group, overlay.Selector)
			}
		}
		if err := checkSoftware(phase.Software[group.Group]); err != nil {
			return err
		}
	}
	return nil
}

// Profile describes the real capture, independent of the replay host OS.
type Profile struct {
	OS                  string   `json:"os"`
	Architecture        string   `json:"architecture"`
	MemoryBytes         uint64   `json:"memory_bytes"`
	Streams             []Stream `json:"streams"`
	MetricNames         []string `json:"metric_names"`
	ProcessNames        []string `json:"process_names"`
	SoftwareNames       []string `json:"software_names"`
	ConnectionSelectors []string `json:"connection_selectors"`
}
