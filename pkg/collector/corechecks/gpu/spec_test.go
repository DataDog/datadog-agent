// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
)

// specFile is the YAML metric specification
type specFile struct {
	Namespace  string          `yaml:"namespace"`
	Collectors []specCollector `yaml:"collectors"`
	Deprecated []struct {
		Name       string `yaml:"name"`
		ReplacedBy string `yaml:"replaced_by"`
	} `yaml:"deprecated_metrics"`
}

type specCollector struct {
	Name     string       `yaml:"name"`
	CodeName string       `yaml:"code_name"`
	Metrics  []specMetric `yaml:"metrics"`
}

type specMetric struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	Priority   string   `yaml:"priority"`
	PerProcess bool     `yaml:"per_process"`
	DedupGroup string   `yaml:"dedup_group"`
	CustomTags []string `yaml:"custom_tags"`
}

// collectedMetric represents a metric collected from a specific collector
type collectedMetric struct {
	name         string
	priority     nvidia.MetricPriority
	collector    string
	hasWorkloads bool
	hasTags      bool
}

func loadSpec(t *testing.T) *specFile {
	t.Helper()
	data, err := os.ReadFile("spec/gpu_metrics.yaml")
	require.NoError(t, err, "failed to read spec file")

	var spec specFile
	require.NoError(t, yaml.Unmarshal(data, &spec))
	return &spec
}
