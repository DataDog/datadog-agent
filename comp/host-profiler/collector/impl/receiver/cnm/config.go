// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	defaultMaxTrackedConnections = 65536
	defaultMaxConnsPerMessage    = 1000
	defaultCheckInterval         = 30 * time.Second
	defaultTCPConnTimeout        = 2 * time.Minute
	defaultUDPConnTimeout        = 30 * time.Second
	defaultUDPStreamTimeout      = 120 * time.Second
	defaultClientStateExpiry     = 2 * time.Minute
	defaultMaxDNSStats           = 20000
	defaultMaxDNSStatsBuffered   = 75000
	defaultDNSTimeout            = 15 * time.Second
	defaultConntrackMaxStateSize = 65536
	defaultConntrackRateLimit    = 500
)

// Config is the configuration for the CNM receiver.
type Config struct {
	CollectTCPv4           bool          `mapstructure:"collect_tcp_v4"`
	CollectTCPv6           bool          `mapstructure:"collect_tcp_v6"`
	CollectUDPv4           bool          `mapstructure:"collect_udp_v4"`
	CollectUDPv6           bool          `mapstructure:"collect_udp_v6"`
	DNSInspection          bool          `mapstructure:"dns_inspection"`
	CollectDNSStats        bool          `mapstructure:"collect_dns_stats"`
	MaxTrackedConnections  int           `mapstructure:"max_tracked_connections"`
	MaxConnsPerMessage     int           `mapstructure:"max_conns_per_message"`
	CheckInterval          time.Duration `mapstructure:"check_interval"`
	ProtocolClassification bool          `mapstructure:"protocol_classification"`
	EnableConntrack        bool          `mapstructure:"enable_conntrack"`
}

var _ xconfmap.Validator = (*Config)(nil)

// Validate checks the CNM receiver configuration for errors.
func (c *Config) Validate() error {
	if c.MaxTrackedConnections <= 0 {
		return errors.New("max_tracked_connections must be positive")
	}
	if c.MaxConnsPerMessage <= 0 {
		return errors.New("max_conns_per_message must be positive")
	}
	if c.CheckInterval <= 0 {
		return errors.New("check_interval must be positive")
	}
	return nil
}

// toNetworkConfig constructs a pkg/network/config.Config from the OTel receiver config.
// This bypasses networkconfig.New() which reads from the global SystemProbe config.
func (c *Config) toNetworkConfig() *networkconfig.Config {
	return &networkconfig.Config{
		Config: *ebpf.NewConfig(),

		NPMEnabled: true,

		CollectTCPv4Conns: c.CollectTCPv4,
		CollectTCPv6Conns: c.CollectTCPv6,
		CollectUDPv4Conns: c.CollectUDPv4,
		CollectUDPv6Conns: c.CollectUDPv6,

		TCPConnTimeout:   defaultTCPConnTimeout,
		UDPConnTimeout:   defaultUDPConnTimeout,
		UDPStreamTimeout: defaultUDPStreamTimeout,

		MaxTrackedConnections:        uint32(c.MaxTrackedConnections),
		MaxClosedConnectionsBuffered: uint32(c.MaxTrackedConnections),
		MaxConnectionsStateBuffered:  int(c.MaxTrackedConnections),
		ClientStateExpiry:            defaultClientStateExpiry,

		DNSInspection:         c.DNSInspection,
		CollectDNSStats:       c.CollectDNSStats,
		MaxDNSStats:           defaultMaxDNSStats,
		MaxDNSStatsBuffered:   defaultMaxDNSStatsBuffered,
		DNSTimeout:            defaultDNSTimeout,
		DNSMonitoringPortList: []int{53},

		ProtocolClassificationEnabled: c.ProtocolClassification,

		EnableConntrack:              c.EnableConntrack,
		ConntrackMaxStateSize:        defaultConntrackMaxStateSize,
		ConntrackRateLimit:           defaultConntrackRateLimit,
		ConntrackRateLimitInterval:   3 * time.Second,
		EnableConntrackAllNamespaces: true,
		EnableEbpfConntracker:        true,

		EnableGatewayLookup: false,
		EnableRootNetNs:     true,
		EnableCORETracer:    true,
		EnableFentry:        true,
	}
}

func defaultConfig() component.Config {
	return &Config{
		CollectTCPv4:           true,
		CollectTCPv6:           true,
		CollectUDPv4:           true,
		CollectUDPv6:           true,
		DNSInspection:          true,
		CollectDNSStats:        true,
		MaxTrackedConnections:  defaultMaxTrackedConnections,
		MaxConnsPerMessage:     defaultMaxConnsPerMessage,
		CheckInterval:          defaultCheckInterval,
		ProtocolClassification: true,
		EnableConntrack:        true,
	}
}

func castConfig(baseCfg component.Config) (*Config, error) {
	cfg, ok := baseCfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *Config, got %T", baseCfg)
	}
	return cfg, nil
}
