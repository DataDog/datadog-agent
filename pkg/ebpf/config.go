package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Config stores all common flags used by system-probe
type Config struct {
	// BPFDebug enables bpf debug logs
	BPFDebug bool

	// BPFDir is the directory to load the eBPF program from
	BPFDir string

	// ProcRoot is the root path to the proc filesystem
	ProcRoot string

	// DebugPort specifies a port to run golang's expvar and pprof debug endpoint
	DebugPort int

	// EnableTracepoints enables use of tracepoints instead of kprobes for probing syscalls (if available on system)
	EnableTracepoints bool
}

// NewDefaultConfig creates a instance of Config with sane default values
func NewDefaultConfig() *Config {
	return &Config{
		BPFDir:   "build",
		BPFDebug: false,
		ProcRoot: "/proc",
	}
}

// SysProbeConfigFromConfig creates a Config from values provided by config.AgentConfig
func SysProbeConfigFromConfig(cfg *config.AgentConfig) *Config {
	ebpfConfig := NewDefaultConfig()

	ebpfConfig.ProcRoot = util.GetProcRoot()
	ebpfConfig.BPFDebug = cfg.SysProbeBPFDebug
	ebpfConfig.BPFDir = cfg.SystemProbeBPFDir
	ebpfConfig.EnableTracepoints = cfg.EnableTracepoints

	return ebpfConfig
}
