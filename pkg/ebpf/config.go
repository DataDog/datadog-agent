package ebpf

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

func curDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}

// NewDefaultConfig creates a instance of Config with sane default values
func NewDefaultConfig() *Config {
	cwd, err := curDir()
	if err != nil {
		// default to repo structure and hope we are running from the root
		cwd = "pkg/ebpf"
	}

	return &Config{
		BPFDir:                   filepath.Join(cwd, "bytecode/build"),
		BPFDebug:                 false,
		ProcRoot:                 "/proc",
		EnableRuntimeCompiler:    false,
		RuntimeCompilerOutputDir: "/var/tmp/datadog-agent/system-probe/build",
	}
}

// SysProbeConfigFromConfig creates a Config from values provided by config.AgentConfig
func SysProbeConfigFromConfig(cfg *config.AgentConfig) *Config {
	ebpfConfig := NewDefaultConfig()

	ebpfConfig.ProcRoot = util.GetProcRoot()
	if cfg != nil {
		ebpfConfig.BPFDebug = cfg.SysProbeBPFDebug
		ebpfConfig.BPFDir = cfg.SystemProbeBPFDir
		ebpfConfig.EnableTracepoints = cfg.EnableTracepoints
		ebpfConfig.EnableRuntimeCompiler = cfg.EnableRuntimeCompiler
		ebpfConfig.KernelHeadersDirs = cfg.KernelHeadersDirs
		ebpfConfig.RuntimeCompilerOutputDir = cfg.RuntimeCompilerOutputDir
	}

	return ebpfConfig
}
