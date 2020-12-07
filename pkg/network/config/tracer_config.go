package config

import (
	"io/ioutil"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/ebpf"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerConfigFromConfig returns a valid tracer-bpf config sourced from our agent config
func TracerConfigFromConfig(cfg *config.AgentConfig) *Config {
	tracerConfig := NewDefaultConfig()
	tracerConfig.Config = *ebpf.SysProbeConfigFromConfig(cfg)

	if !isIPv6EnabledOnHost() {
		tracerConfig.CollectIPv6Conns = false
		log.Info("system probe IPv6 tracing disabled by system")
	} else if cfg.DisableIPv6Tracing {
		tracerConfig.CollectIPv6Conns = false
		log.Info("system probe IPv6 tracing disabled by configuration")
	}

	if cfg.DisableUDPTracing {
		tracerConfig.CollectUDPConns = false
		log.Info("system probe UDP tracing disabled by configuration")
	}

	if cfg.DisableTCPTracing {
		tracerConfig.CollectTCPConns = false
		log.Info("system probe TCP tracing disabled by configuration")
	}

	if cfg.DisableDNSInspection {
		tracerConfig.DNSInspection = false
		log.Info("system probe DNS inspection disabled by configuration")
	}

	if len(cfg.ExcludedSourceConnections) > 0 {
		tracerConfig.ExcludedSourceConnections = cfg.ExcludedSourceConnections
	}

	if len(cfg.ExcludedDestinationConnections) > 0 {
		tracerConfig.ExcludedDestinationConnections = cfg.ExcludedDestinationConnections
	}

	tracerConfig.CollectLocalDNS = cfg.CollectLocalDNS
	tracerConfig.CollectDNSStats = cfg.CollectDNSStats

	if to := cfg.DNSTimeout; to > 0 {
		tracerConfig.DNSTimeout = cfg.DNSTimeout
	}

	tracerConfig.MaxTrackedConnections = cfg.MaxTrackedConnections
	tracerConfig.EnableConntrack = cfg.EnableConntrack
	tracerConfig.ConntrackMaxStateSize = cfg.ConntrackMaxStateSize
	tracerConfig.EnableConntrackAllNamespaces = cfg.EnableConntrackAllNamespaces
	tracerConfig.DebugPort = cfg.SystemProbeDebugPort

	if mccb := cfg.MaxClosedConnectionsBuffered; mccb > 0 {
		tracerConfig.MaxClosedConnectionsBuffered = mccb
	}

	if mcsb := cfg.MaxConnectionsStateBuffered; mcsb > 0 {
		tracerConfig.MaxConnectionsStateBuffered = mcsb
	}

	if ccs := cfg.ClosedChannelSize; ccs > 0 {
		tracerConfig.ClosedChannelSize = ccs
	}

	if th := cfg.OffsetGuessThreshold; th > 0 {
		tracerConfig.OffsetGuessThreshold = th
	}

	tracerConfig.EnableMonotonicCount = cfg.Windows.EnableMonotonicCount
	tracerConfig.DriverBufferSize = cfg.Windows.DriverBufferSize

	return tracerConfig
}

func isIPv6EnabledOnHost() bool {
	_, err := ioutil.ReadFile(filepath.Join(util.GetProcRoot(), "net/if_inet6"))
	return err == nil
}
