// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// NetflowConfig contains configuration for NetFlow collector.
type NetflowConfig struct {
	Enabled                       bool             `mapstructure:"enabled"`
	Listeners                     []ListenerConfig `mapstructure:"listeners"`
	StopTimeout                   int              `mapstructure:"stop_timeout"`
	AggregatorBufferSize          int              `mapstructure:"aggregator_buffer_size"`
	AggregatorFlushInterval       int              `mapstructure:"aggregator_flush_interval"`
	AggregatorFlowContextTTL      int              `mapstructure:"aggregator_flow_context_ttl"`
	AggregatorPortRollupThreshold int              `mapstructure:"aggregator_port_rollup_threshold"`
	AggregatorPortRollupDisabled  bool             `mapstructure:"aggregator_port_rollup_disabled"`

	// AggregatorRollupTrackerRefreshInterval is useful to speed up testing to avoid wait for 1h default
	AggregatorRollupTrackerRefreshInterval uint `mapstructure:"aggregator_rollup_tracker_refresh_interval"`

	PrometheusListenerAddress string `mapstructure:"prometheus_listener_address"` // Example `localhost:9090`
	PrometheusListenerEnabled bool   `mapstructure:"prometheus_listener_enabled"`

	ReverseDNSEnrichmentEnabled bool `mapstructure:"reverse_dns_enrichment_enabled"`
}

// ListenerConfig contains configuration for a single flow listener
type ListenerConfig struct {
	FlowType  common.FlowType `mapstructure:"flow_type"`
	Port      uint16          `mapstructure:"port"`
	BindHost  string          `mapstructure:"bind_host"`
	Workers   int             `mapstructure:"workers"`
	Namespace string          `mapstructure:"namespace"`
	Mapping   []Mapping       `mapstructure:"mapping"`
}

// Mapping contains configuration for a Netflow/IPFIX field mapping
type Mapping struct {
	Field       uint16            `mapstructure:"field"`
	Destination string            `mapstructure:"destination"`
	Endian      common.EndianType `mapstructure:"endianness"`
	Type        common.FieldType  `mapstructure:"type"`
}

// Addr returns the host:port address to listen on.
func (c *ListenerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}
