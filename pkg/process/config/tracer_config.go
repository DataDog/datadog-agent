package config

import (
	"io/ioutil"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// SysProbeConfigFromConfig returns a valid tracer-bpf config sourced from our agent config
func SysProbeConfigFromConfig(cfg *AgentConfig) *network.Config {
	tracerConfig := network.NewDefaultConfig()

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

	tracerConfig.MaxTrackedConnections = cfg.MaxTrackedConnections
	tracerConfig.ProcRoot = util.GetProcRoot()
	tracerConfig.BPFDebug = cfg.SysProbeBPFDebug
	tracerConfig.EnableConntrack = cfg.EnableConntrack
	tracerConfig.ConntrackShortTermBufferSize = cfg.ConntrackShortTermBufferSize
	tracerConfig.ConntrackMaxStateSize = cfg.ConntrackMaxStateSize
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

	return tracerConfig
}

func isIPv6EnabledOnHost() bool {
	_, err := ioutil.ReadFile(filepath.Join(util.GetProcRoot(), "net/if_inet6"))
	return err == nil
}
