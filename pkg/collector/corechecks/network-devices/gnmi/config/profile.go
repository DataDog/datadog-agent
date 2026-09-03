// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const profilesFolder = "profiles"

// MetricType is the Datadog metric submission type for a gNMI metric mapping.
type MetricType string

const (
	// MetricTypeGauge maps to Datadog gauge metrics.
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeMonotonicCount maps to Datadog monotonic count metrics.
	MetricTypeMonotonicCount MetricType = "monotonic_count"
)

var supportedMetricTypes = map[MetricType]struct{}{
	MetricTypeGauge:          {},
	MetricTypeMonotonicCount: {},
}

// MetricConfig maps a gNMI path to a Datadog metric.
type MetricConfig struct {
	Path     string            `yaml:"path"`
	Metric   string            `yaml:"metric"`
	Type     MetricType        `yaml:"type"`
	Tags     map[string]string `yaml:"tags"`
	ValueMap map[string]int    `yaml:"value_map"`
}

// ProfileDefinition holds the metric mappings loaded from a profile YAML file.
type ProfileDefinition struct {
	Name    string
	Path    string
	Metrics []MetricConfig `yaml:"metrics"`
}

type rawProfileDefinition struct {
	Metrics []MetricConfig `yaml:"metrics"`
}

// LoadProfile loads a profile by name or path from the gNMI profiles directory.
func LoadProfile(profileRef string) (*ProfileDefinition, error) {
	profilePath, err := resolveProfilePath(profileRef)
	if err != nil {
		return nil, err
	}

	buf, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", profileRef, err)
	}

	raw := rawProfileDefinition{}
	if err := yaml.Unmarshal(buf, &raw); err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", profileRef, err)
	}

	profile := &ProfileDefinition{
		Name:    profileNameFromPath(profileRef, profilePath),
		Path:    profilePath,
		Metrics: raw.Metrics,
	}
	if err := validateProfileDefinition(profile); err != nil {
		return nil, fmt.Errorf("validate profile %q: %w", profileRef, err)
	}

	return profile, nil
}

func validateProfileDefinition(profile *ProfileDefinition) error {
	if len(profile.Metrics) == 0 {
		return errors.New("profile must define at least one metric")
	}

	for i, metric := range profile.Metrics {
		if err := validateMetricConfig(metric, i); err != nil {
			return err
		}
	}
	return nil
}

func validateMetricConfig(metric MetricConfig, index int) error {
	metricPrefix := fmt.Sprintf("metrics[%d]", index)

	if strings.TrimSpace(metric.Path) == "" {
		return fmt.Errorf("%s: `path` must not be empty", metricPrefix)
	}
	if strings.TrimSpace(metric.Metric) == "" {
		return fmt.Errorf("%s: `metric` must not be empty", metricPrefix)
	}
	if _, ok := supportedMetricTypes[metric.Type]; !ok {
		return fmt.Errorf("%s: unsupported metric `type` %q (supported: gauge, monotonic_count)", metricPrefix, metric.Type)
	}
	return nil
}

func resolveProfilePath(profileRef string) (string, error) {
	if profileRef == "" {
		return "", errors.New("profile reference is empty")
	}

	if filepath.IsAbs(profileRef) {
		return profileRef, nil
	}

	profilesRoot := getProfilesRoot()
	candidate := filepath.Join(profilesRoot, profileRef)
	if fileExists(candidate) {
		return candidate, nil
	}

	withYAMLExt := ensureYAMLExtension(profileRef)
	candidate = filepath.Join(profilesRoot, withYAMLExt)
	if fileExists(candidate) {
		return candidate, nil
	}

	return "", fmt.Errorf("profile %q not found under %q", profileRef, profilesRoot)
}

func getProfilesRoot() string {
	confdPath := pkgconfigsetup.Datadog().GetString("confd_path")
	return filepath.Join(confdPath, "gnmi.d", profilesFolder)
}

func ensureYAMLExtension(profileRef string) string {
	if strings.HasSuffix(profileRef, ".yaml") || strings.HasSuffix(profileRef, ".yml") {
		return profileRef
	}
	return profileRef + ".yaml"
}

func profileNameFromPath(profileRef, profilePath string) string {
	base := filepath.Base(profilePath)
	if strings.HasSuffix(base, ".yaml") {
		return strings.TrimSuffix(base, ".yaml")
	}
	if strings.HasSuffix(base, ".yml") {
		return strings.TrimSuffix(base, ".yml")
	}
	return profileRef
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
