// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package schema defines the portable simulator scenario and run contracts.
package schema

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	implicitGroupName = "endpoints"
	implicitPhaseName = "healthy"
)

// Scenario is the normalized simulator scenario. The YAML contract has a
// compact replay form for a single cohort and retains the explicit form for
// scenarios that need named cohorts, expectations, phases, or overlays.
type Scenario struct {
	Version         int                       `yaml:"version"`
	Expectation     Expectation               `yaml:"expectation"`
	MonitorWindow   Duration                  `yaml:"monitor_window"`
	VisibilityDelay Duration                  `yaml:"visibility_delay"`
	Meta            ScenarioMeta              `yaml:"scenario"`
	Fleet           []GroupDef                `yaml:"fleet"`
	Software        map[string][]SoftwareItem `yaml:"software_inventory"`
	Phases          []Phase                   `yaml:"phases"`
	NetworkDevices  NetworkDevicesConfig      `yaml:"network_devices"` // optional: simulated NDM access points
}

// Replay defines the duration of a compact, overlay-free replay.
type Replay struct {
	Duration Duration `yaml:"duration"`
}

type compactFleet struct {
	Count int    `yaml:"count"`
	OS    string `yaml:"os"`
}

type scenarioFleet struct {
	groups  []GroupDef
	compact bool
}

func (f *scenarioFleet) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		var fleet compactFleet
		if err := decodeNodeStrict(value, &fleet); err != nil {
			return err
		}
		f.groups = []GroupDef{{Group: implicitGroupName, Count: fleet.Count, OS: fleet.OS}}
		f.compact = true
		return nil
	case yaml.SequenceNode:
		type groups []GroupDef
		var fleet groups
		if err := decodeNodeStrict(value, &fleet); err != nil {
			return err
		}
		f.groups = []GroupDef(fleet)
		return nil
	default:
		return errors.New("fleet must be a count/os mapping or a list of cohorts")
	}
}

type scenarioDocument struct {
	Version         int                       `yaml:"version"`
	Expectation     *Expectation              `yaml:"expectation"`
	MonitorWindow   *Duration                 `yaml:"monitor_window"`
	VisibilityDelay *Duration                 `yaml:"visibility_delay"`
	Meta            ScenarioMeta              `yaml:"scenario"`
	Fleet           scenarioFleet             `yaml:"fleet"`
	Replay          *Replay                   `yaml:"replay"`
	Software        map[string][]SoftwareItem `yaml:"software_inventory"`
	Phases          *[]Phase                  `yaml:"phases"`
	NetworkDevices  *NetworkDevicesConfig     `yaml:"network_devices"`
}

// UnmarshalYAML normalizes the compact replay form into the same internal
// cohort and phase model used by advanced scenarios. Keeping one execution
// model avoids giving the simple syntax subtly different replay behavior.
func (s *Scenario) UnmarshalYAML(value *yaml.Node) error {
	var document scenarioDocument
	if err := decodeNodeStrict(value, &document); err != nil {
		return err
	}
	if document.Replay != nil && document.Phases != nil {
		return errors.New("replay and phases are mutually exclusive")
	}
	if document.Fleet.compact && document.Replay == nil {
		return errors.New("compact fleet requires replay")
	}
	if !document.Fleet.compact && document.Replay != nil {
		return errors.New("replay requires compact fleet")
	}
	if document.Fleet.compact && (document.Expectation != nil || document.MonitorWindow != nil || document.VisibilityDelay != nil || document.Software != nil || document.NetworkDevices != nil) {
		return errors.New("compact replay accepts only version, scenario, fleet, and replay")
	}

	var expectation Expectation
	if document.Expectation != nil {
		expectation = *document.Expectation
	}
	var monitorWindow, visibilityDelay Duration
	if document.MonitorWindow != nil {
		monitorWindow = *document.MonitorWindow
	}
	if document.VisibilityDelay != nil {
		visibilityDelay = *document.VisibilityDelay
	}
	var phases []Phase
	if document.Phases != nil {
		phases = *document.Phases
	}
	var networkDevices NetworkDevicesConfig
	if document.NetworkDevices != nil {
		networkDevices = *document.NetworkDevices
	}
	if document.Replay != nil {
		expectation = Expectation{Conclusion: Healthy}
		phases = []Phase{{Name: implicitPhaseName, Duration: document.Replay.Duration}}
	}
	*s = Scenario{
		Version:         document.Version,
		Expectation:     expectation,
		MonitorWindow:   monitorWindow,
		VisibilityDelay: visibilityDelay,
		Meta:            document.Meta,
		Fleet:           document.Fleet.groups,
		Software:        document.Software,
		Phases:          phases,
		NetworkDevices:  networkDevices,
	}
	return nil
}

// ScenarioMeta holds scenario identity fields.
type ScenarioMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// GroupDef defines a fleet group (a set of devices with shared characteristics).
type GroupDef struct {
	Group      string   `yaml:"group"`
	Count      int      `yaml:"count"`
	OS         string   `yaml:"os"` // macos | windows
	Tags       []string `yaml:"tags"`
	TotalRAMGB int      `yaml:"total_ram_gb"`
	SSID       string   `yaml:"ssid"`  // optional shared SSID; empty preserves captured association
	BSSID      string   `yaml:"bssid"` // optional shared BSSID; empty preserves captured association
	// AccessPoint/Radio are mutually exclusive with BSSID — the network_devices path.
	AccessPoint      string  `yaml:"access_point"`      // name of a network_devices.access_points entry this group associates to
	Radio            string  `yaml:"radio"`             // radio interface name on AccessPoint; default = AccessPoint's first radio
	BaselineVariance float64 `yaml:"baseline_variance"` // 0.0 = off (default), 0.15 = ±15% spread
}

// SoftwareItem represents an installed application.
type SoftwareItem struct {
	Name             string `yaml:"name"`
	Version          string `yaml:"version"`
	Publisher        string `yaml:"publisher"`
	SoftwareType     string `yaml:"software_type"`          // macOS: "app"/"homebrew"/"pkg"; Windows: "desktop"/"msstore"
	DeploymentStatus string `yaml:"deployment_status"`      // empty preserves captured status
	Is64Bit          bool   `yaml:"is_64_bit"`              // true sets the flag; false preserves capture
	DeploymentTime   string `yaml:"deployment_time"`        // explicit installation date; empty preserves captured capture
	ProductCode      string `yaml:"product_code,omitempty"` // e.g. "{GUID}"; empty preserves capture before identity rewriting
	User             string `yaml:"user,omitempty"`         // Windows per-user SID or account; empty preserves capture
}

// Phase defines a time window with process and metric behavior per group.
type Phase struct {
	Name           string                         `yaml:"name"`
	Duration       Duration                       `yaml:"duration"`
	JitterScale    float64                        `yaml:"jitter_scale"` // multiplier on all jitter, default 1.0
	Processes      map[string][]ProcessDef        `yaml:"processes"`
	Metrics        map[string]map[string]Pattern  `yaml:"metrics"`
	Connections    map[string][]ConnectionOverlay `yaml:"connections"`
	Software       map[string][]SoftwareItem      `yaml:"software_inventory"`
	NetworkMetrics map[string]APPhaseMetrics      `yaml:"network_metrics"` // optional: access point name → AP/interface metrics
}

// ProcessDef defines a named process within a group for a phase.
// Every named process must exist in the baseline capture. Unspecified
// background processes retain their captured values.
type ProcessDef struct {
	Name   string   `yaml:"name"`
	User   string   `yaml:"user,omitempty"` // OS user; empty preserves the captured user
	Exe    string   `yaml:"exe,omitempty"`  // full executable path; empty preserves capture
	Args   []string `yaml:"args,omitempty"` // full argv; omitted preserves capture; argv[0] follows the executable
	CPU    Pattern  `yaml:"cpu"`            // whole-host percent (0-100); converted to Agent per-core process accounting
	Memory Pattern  `yaml:"memory"`         // MB
}

// Pattern defines how a metric value evolves over a phase.
// Exactly one field should be set.
type Pattern struct {
	Steady *SteadyPattern `yaml:"steady,omitempty"`
	Ramp   *RampPattern   `yaml:"ramp,omitempty"`
	Spike  *SpikePattern  `yaml:"spike,omitempty"`
	Step   *StepPattern   `yaml:"step,omitempty"`
}

// SteadyPattern holds a constant base value (±jitter applied at evaluation time).
type SteadyPattern struct {
	Value float64 `yaml:"value"`
}

// RampPattern linearly interpolates from From to To over the phase duration.
type RampPattern struct {
	From float64 `yaml:"from"`
	To   float64 `yaml:"to"`
}

// SpikePattern produces a Gaussian peak at position At (0-100%) of the phase,
// with the peak spanning Duration (0-100%) of the phase width.
type SpikePattern struct {
	Baseline float64 `yaml:"baseline"`
	Peak     float64 `yaml:"peak"`
	At       float64 `yaml:"at"`       // percent of phase (0-100)
	Duration float64 `yaml:"duration"` // percent of phase (0-100)
}

// StepPattern abruptly transitions from Before to After at position At (0-100%) of the phase.
type StepPattern struct {
	Before float64 `yaml:"before"`
	After  float64 `yaml:"after"`
	At     float64 `yaml:"at"` // percent of phase (0-100)
}

// Duration is a time.Duration that unmarshals from YAML strings like "10m", "5s".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}

// UnmarshalYAML handles the compact pattern syntax.
// Supports: {steady: N}, {ramp: {from: N, to: M}}, etc.
// Also handles the shorthand where a bare number is treated as steady.
func (p *Pattern) UnmarshalYAML(value *yaml.Node) error {
	// Try to decode as a bare number (shorthand for steady)
	var num float64
	if err := value.Decode(&num); err == nil {
		*p = Pattern{Steady: &SteadyPattern{Value: num}}
		return nil
	}

	// Full struct decode
	type patternAlias Pattern
	var alias patternAlias
	if err := decodeNodeStrict(value, &alias); err != nil {
		return err
	}
	*p = Pattern(alias)
	return nil
}

// UnmarshalYAML handles compact steady syntax: {steady: N} where N is a number directly.
func (s *SteadyPattern) UnmarshalYAML(value *yaml.Node) error {
	// Handle: steady: 8  (scalar)
	var num float64
	if err := value.Decode(&num); err == nil {
		s.Value = num
		return nil
	}
	// Handle: steady: {value: 8}  (mapping)
	type alias SteadyPattern
	var a alias
	if err := decodeNodeStrict(value, &a); err != nil {
		return err
	}
	*s = SteadyPattern(a)
	return nil
}

// Load reads and parses a scenario YAML file.
func Load(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario file: %w", err)
	}
	var s Scenario
	if err := DecodeStrict(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario YAML: %w", err)
	}
	return &s, nil
}

// GroupByName returns the GroupDef for the given group name, or nil.
func (s *Scenario) GroupByName(name string) *GroupDef {
	for i := range s.Fleet {
		if s.Fleet[i].Group == name {
			return &s.Fleet[i]
		}
	}
	return nil
}

// EffectiveSSID returns the SSID for a group, defaulting to "Corp-WiFi".
func (g *GroupDef) EffectiveSSID() string {
	if g.SSID != "" {
		return g.SSID
	}
	return "Corp-WiFi"
}

// EffectiveJitterScale returns the jitter scale for a phase, defaulting to 1.0.
func (p *Phase) EffectiveJitterScale() float64 {
	if p.JitterScale == 0 {
		return 1.0
	}
	return p.JitterScale
}

// AccessPointByName returns the declared access point with the given name, or nil.
func (s *Scenario) AccessPointByName(name string) *AccessPointDef {
	for i := range s.NetworkDevices.AccessPoints {
		if s.NetworkDevices.AccessPoints[i].Name == name {
			return &s.NetworkDevices.AccessPoints[i]
		}
	}
	return nil
}

// EffectiveIntegration returns the NDM integration name for this scenario's
// network devices, defaulting to "snmp".
func (s *Scenario) EffectiveIntegration() string {
	if s.NetworkDevices.Integration != "" {
		return s.NetworkDevices.Integration
	}
	return "snmp"
}

// DecodeStrict rejects unknown fields and trailing documents. This also applies
// inside custom pattern decoders, where yaml.Node.Decode alone is permissive.
func DecodeStrict(data []byte, dst any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one YAML document")
	}
	return nil
}

func decodeNodeStrict(node *yaml.Node, dst any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	return DecodeStrict(data, dst)
}
