// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "go.yaml.in/yaml/v2"
)

// defaultTimeout is the per-run timeout, in seconds, applied to each cmdlet
// invocation when the instance does not override it.
const defaultTimeout = 30

// validMetricTypes is the set of metric types the check can submit. An empty
// type defaults to "gauge".
var validMetricTypes = map[string]struct{}{
	"gauge":           {},
	"rate":            {},
	"count":           {},
	"monotonic_count": {},
	"histogram":       {},
	"distribution":    {},
}

// metricEntry maps a cmdlet output property to a Datadog metric.
//
// It accepts two YAML forms (the second is the extensible one, see RFC §8.1):
//
//   - [Property, metric_name, type]          # positional tuple (WMI-parity)
//   - {property: Property, name: ..., type:} # mapping form
//
// Property may be the literal "1" for a "virtual" metric whose value is a
// constant 1 and whose tags carry the signal (the all-strings case).
type metricEntry struct {
	Property string
	Name     string
	Type     string
}

// UnmarshalYAML implements dual (positional tuple / mapping) parsing.
func (m *metricEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var seq []interface{}
	if err := unmarshal(&seq); err == nil && len(seq) > 0 {
		if len(seq) < 2 || len(seq) > 3 {
			return fmt.Errorf("metric tuple must be [property, name] or [property, name, type], got %d elements", len(seq))
		}
		m.Property = scalarToString(seq[0])
		m.Name = scalarToString(seq[1])
		if len(seq) == 3 {
			m.Type = scalarToString(seq[2])
		}
		return m.finalize()
	}

	var mp struct {
		Property interface{} `yaml:"property"`
		Name     string      `yaml:"name"`
		Type     string      `yaml:"type"`
	}
	if err := unmarshal(&mp); err != nil {
		return err
	}
	m.Property = scalarToString(mp.Property)
	m.Name = mp.Name
	m.Type = mp.Type
	return m.finalize()
}

func (m *metricEntry) finalize() error {
	if m.Property == "" {
		return fmt.Errorf("metric %q is missing a property", m.Name)
	}
	if m.Name == "" {
		return fmt.Errorf("metric for property %q is missing a name", m.Property)
	}
	if m.Type == "" {
		m.Type = "gauge"
	}
	if _, ok := validMetricTypes[m.Type]; !ok {
		return fmt.Errorf("metric %q has invalid type %q", m.Name, m.Type)
	}
	return nil
}

// isVirtual reports whether the metric is a constant-1 "virtual" metric.
func (m *metricEntry) isVirtual() bool {
	return m.Property == "1"
}

// filterEntry is a cmdlet named parameter and its value. It is bound (splatted)
// to the cmdlet as data, never interpolated into the command text.
//
// Accepts [Name, Value] or {name: Name, value: Value}.
type filterEntry struct {
	Name  string
	Value interface{}
}

// UnmarshalYAML implements dual (positional tuple / mapping) parsing.
func (f *filterEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var seq []interface{}
	if err := unmarshal(&seq); err == nil && len(seq) > 0 {
		if len(seq) != 2 {
			return fmt.Errorf("filter tuple must be [Name, Value], got %d elements", len(seq))
		}
		f.Name = scalarToString(seq[0])
		f.Value = seq[1]
		return f.finalize()
	}

	var mp struct {
		Name  string      `yaml:"name"`
		Value interface{} `yaml:"value"`
	}
	if err := unmarshal(&mp); err != nil {
		return err
	}
	f.Name = mp.Name
	f.Value = mp.Value
	return f.finalize()
}

func (f *filterEntry) finalize() error {
	if f.Name == "" {
		return errors.New("filter is missing a parameter name")
	}
	return nil
}

// tagByEntry adds a per-row tag from a cmdlet output property, with an optional
// alias. Accepts the strings "Property" or "Property AS alias".
type tagByEntry struct {
	Property string
	Alias    string
}

// UnmarshalYAML parses the "Property [AS alias]" string form (or a mapping).
func (t *tagByEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		prop, alias := splitAS(s)
		t.Property = prop
		t.Alias = alias
		return t.finalize()
	}

	var mp struct {
		Property string `yaml:"property"`
		Alias    string `yaml:"alias"`
	}
	if err := unmarshal(&mp); err != nil {
		return err
	}
	t.Property = mp.Property
	t.Alias = mp.Alias
	return t.finalize()
}

func (t *tagByEntry) finalize() error {
	if t.Property == "" {
		return errors.New("tag_by entry is missing a property")
	}
	if t.Alias == "" {
		t.Alias = strings.ToLower(t.Property)
	}
	return nil
}

// tagQueryEntry joins this cmdlet's rows to another Get-* cmdlet's output to
// add a tag. Accepts the positional form:
//
//	[LinkSourceProperty, TargetCmdlet, LinkTargetProperty, "TargetProperty [AS alias]"]
type tagQueryEntry struct {
	LinkSourceProperty string
	TargetCmdlet       string
	LinkTargetProperty string
	TargetProperty     string
	Alias              string
}

// UnmarshalYAML parses the positional tuple form.
func (q *tagQueryEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var seq []interface{}
	if err := unmarshal(&seq); err == nil && len(seq) > 0 {
		if len(seq) != 4 {
			return fmt.Errorf("tag_queries tuple must be [LinkSource, TargetCmdlet, LinkTarget, TargetProperty], got %d elements", len(seq))
		}
		q.LinkSourceProperty = scalarToString(seq[0])
		q.TargetCmdlet = scalarToString(seq[1])
		q.LinkTargetProperty = scalarToString(seq[2])
		q.TargetProperty, q.Alias = splitAS(scalarToString(seq[3]))
		return q.finalize()
	}

	var mp struct {
		LinkSourceProperty string `yaml:"link_source_property"`
		TargetCmdlet       string `yaml:"target_cmdlet"`
		LinkTargetProperty string `yaml:"link_target_property"`
		TargetProperty     string `yaml:"target_property"`
		Alias              string `yaml:"alias"`
	}
	if err := unmarshal(&mp); err != nil {
		return err
	}
	q.LinkSourceProperty = mp.LinkSourceProperty
	q.TargetCmdlet = mp.TargetCmdlet
	q.LinkTargetProperty = mp.LinkTargetProperty
	q.TargetProperty = mp.TargetProperty
	q.Alias = mp.Alias
	return q.finalize()
}

func (q *tagQueryEntry) finalize() error {
	if q.LinkSourceProperty == "" || q.TargetCmdlet == "" || q.LinkTargetProperty == "" || q.TargetProperty == "" {
		return errors.New("tag_queries entry is missing one of LinkSource/TargetCmdlet/LinkTarget/TargetProperty")
	}
	if q.Alias == "" {
		q.Alias = strings.ToLower(q.TargetProperty)
	}
	return nil
}

// instanceConfig is the per-instance check configuration.
type instanceConfig struct {
	Cmdlet     string          `yaml:"cmdlet"`
	Name       string          `yaml:"name"`
	Filters    []filterEntry   `yaml:"filters"`
	Metrics    []metricEntry   `yaml:"metrics"`
	TagBy      []tagByEntry    `yaml:"tag_by"`
	Tags       []string        `yaml:"tags"`
	TagQueries []tagQueryEntry `yaml:"tag_queries"`
	Timeout    int             `yaml:"timeout"`
}

// parseInstanceConfig unmarshals and validates a single instance's YAML.
func parseInstanceConfig(data []byte) (*instanceConfig, error) {
	var inst instanceConfig
	if err := yaml.Unmarshal(data, &inst); err != nil {
		return nil, err
	}

	if inst.Cmdlet == "" {
		return nil, errors.New("'cmdlet' is required")
	}
	if err := validateGetCmdletName(inst.Cmdlet); err != nil {
		return nil, err
	}
	if len(inst.Metrics) == 0 {
		return nil, errors.New("at least one entry in 'metrics' is required")
	}
	// timeout is optional and defaults to defaultTimeout. A negative value is
	// invalid; warn and fall back to the default rather than failing the check.
	if inst.Timeout < 0 {
		log.Warnf("powershell check: 'timeout' must be a positive number of seconds, got %d; using default of %ds", inst.Timeout, defaultTimeout)
	}
	if inst.Timeout <= 0 {
		inst.Timeout = defaultTimeout
	}
	return &inst, nil
}

// metricName returns the full metric name for a metric entry, applying the
// optional instance name as a prefix. When name is unset the metric name is
// used bare (no forced namespace).
func (c *instanceConfig) metricName(m *metricEntry) string {
	if c.Name == "" {
		return m.Name
	}
	return c.Name + "." + m.Name
}

// selectProperties returns the deduplicated set of properties that must be
// projected with Select-Object for the main cmdlet: every non-virtual metric
// property, every tag_by property, and every tag_queries link-source property.
func (c *instanceConfig) selectProperties() []string {
	seen := make(map[string]struct{})
	var props []string
	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		props = append(props, p)
	}
	for i := range c.Metrics {
		if !c.Metrics[i].isVirtual() {
			add(c.Metrics[i].Property)
		}
	}
	for i := range c.TagBy {
		add(c.TagBy[i].Property)
	}
	for i := range c.TagQueries {
		add(c.TagQueries[i].LinkSourceProperty)
	}
	return props
}

// scalarToString renders a YAML scalar (string, int, float, bool, nil) as a
// string. Used when reading positional-tuple elements.
func scalarToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

// splitAS splits "Property AS alias" (case-insensitive on the AS keyword) into
// its property and alias. When there is no AS clause, the alias defaults to the
// lowercased property.
func splitAS(s string) (property, alias string) {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	for i := 1; i < len(fields)-1; i++ {
		if strings.EqualFold(fields[i], "AS") {
			property = strings.Join(fields[:i], " ")
			alias = strings.Join(fields[i+1:], " ")
			return property, alias
		}
	}
	return s, strings.ToLower(s)
}
