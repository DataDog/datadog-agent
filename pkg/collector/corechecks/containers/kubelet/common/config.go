// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package common provides types used by the Kubelet check.
package common

import (
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
)

const (
	// KubeletMetricsPrefix is the prefix included in the metrics emitted by the kubernetes_core check.
	KubeletMetricsPrefix = "kubernetes."
)

// KubeletConfig is the config of the Kubelet.
type KubeletConfig struct {
	ProbesMetricsEndpoint     *string  `yaml:"probes_metrics_endpoint,omitempty"`
	SlisMetricsEndpoint       *string  `yaml:"slis_metrics_endpoint,omitempty"`
	EnabledRates              []string `yaml:"enabled_rates,omitempty"`
	UseStatsSummaryAsSource   *bool    `yaml:"use_stats_summary_as_source,omitempty"`
	types.OpenmetricsInstance `yaml:",inline"`
}

// Parse parses the configuration.
func (c *KubeletConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}
