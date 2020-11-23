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

	// EnableRuntimeCompiler enables the use of the embedded compiler to build eBPF programs on-host
	EnableRuntimeCompiler bool

	// KernelHeadersDir is the directories of the kernel headers to use for runtime compilation
	KernelHeadersDirs []string

	// RuntimeCompilerOutputDir is the directory where the runtime compiler will store compiled programs
	RuntimeCompilerOutputDir string
}

// NewDefaultConfig creates a instance of Config with sane default values
func NewDefaultConfig() *Config {
	return &Config{
		BPFDir:                   "build",
		BPFDebug:                 false,
		ProcRoot:                 "/proc",
		EnableRuntimeCompiler:    true,
		RuntimeCompilerOutputDir: "/var/tmp/datadog-agent/system-probe/build",
	}
}

// SysProbeConfigFromConfig creates a Config from values provided by config.AgentConfig
func SysProbeConfigFromConfig(cfg *config.AgentConfig) *Config {
	ebpfConfig := NewDefaultConfig()

	ebpfConfig.ProcRoot = util.GetProcRoot()
	ebpfConfig.BPFDebug = cfg.SysProbeBPFDebug
	ebpfConfig.BPFDir = cfg.SystemProbeBPFDir
	ebpfConfig.EnableTracepoints = cfg.EnableTracepoints
	ebpfConfig.EnableRuntimeCompiler = cfg.EnableRuntimeCompiler
	ebpfConfig.KernelHeadersDirs = cfg.KernelHeadersDirs
	ebpfConfig.RuntimeCompilerOutputDir = cfg.RuntimeCompilerOutputDir

	return ebpfConfig
}
