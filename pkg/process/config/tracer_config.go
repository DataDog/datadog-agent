package config

import (
	"io/ioutil"
	"os"
	"path/filepath"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// SysProbeConfigFromConfig returns a valid tracer-bpf config sourced from our agent config
func SysProbeConfigFromConfig(cfg *AgentConfig) *ebpf.Config {
	tracerConfig := ebpf.NewDefaultConfig()

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

	tracerConfig.CollectLocalDNS = cfg.CollectLocalDNS

	tracerConfig.MaxTrackedConnections = cfg.MaxTrackedConnections
	tracerConfig.ProcRoot = getProcRoot()
	tracerConfig.BPFDebug = cfg.SysProbeBPFDebug
	tracerConfig.EnableConntrack = cfg.EnableConntrack
	tracerConfig.ConntrackShortTermBufferSize = cfg.ConntrackShortTermBufferSize

	return tracerConfig
}

func isIPv6EnabledOnHost() bool {
	_, err := ioutil.ReadFile(filepath.Join(getProcRoot(), "net/if_inet6"))
	return err == nil
}

func getProcRoot() string {
	if v := os.Getenv("HOST_PROC"); v != "" {
		return v
	}

	if ddconfig.IsContainerized() && util.PathExists("/host") {
		return "/host/proc"
	}

	return "/proc"
}
