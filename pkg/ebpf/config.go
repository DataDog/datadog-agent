package ebpf

import (
	"strings"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	spNS = "system_probe_config"
)

// Config stores all common flags used by system-probe
type Config struct {
	// BPFDebug enables bpf debug logs
	BPFDebug bool

	// BPFDir is the directory to load the eBPF program from
	BPFDir string

	// ExcludedBPFLinuxVersions lists Linux kernel versions that should not use BPF features
	ExcludedBPFLinuxVersions []string

	// ProcRoot is the root path to the proc filesystem
	ProcRoot string

	// EnableTracepoints enables use of tracepoints instead of kprobes for probing syscalls (if available on system)
	EnableTracepoints bool

	// EnableRuntimeCompiler enables the use of the embedded compiler to build eBPF programs on-host
	EnableRuntimeCompiler bool

	// KernelHeadersDir is the directories of the kernel headers to use for runtime compilation
	KernelHeadersDirs []string

	// RuntimeCompilerOutputDir is the directory where the runtime compiler will store compiled programs
	RuntimeCompilerOutputDir string

	// AllowPrecompiledFallback indicates whether we are allowed to fallback to the prebuilt probes if runtime compilation fails.
	AllowPrecompiledFallback bool
}

//// curDir is used for testing purposes only
//func curDir() (string, error) {
//	_, file, _, ok := runtime.Caller(0)
//	if !ok {
//		return "", fmt.Errorf("unable to get current file build path")
//	}
//
//	buildDir := filepath.Dir(file)
//
//	// build relative path from base of repo
//	buildRoot := rootDir(buildDir)
//	relPath, err := filepath.Rel(buildRoot, buildDir)
//	if err != nil {
//		return "", err
//	}
//
//	cwd, err := os.Getwd()
//	if err != nil {
//		return "", err
//	}
//	curRoot := rootDir(cwd)
//
//	return filepath.Join(curRoot, relPath), nil
//}
//
//// rootDir returns the base repository directory, just before `pkg`.
//// If `pkg` is not found, the dir provided is returned.
//func rootDir(dir string) string {
//	pkgIndex := -1
//	parts := strings.Split(dir, string(filepath.Separator))
//	for i, d := range parts {
//		if d == "pkg" {
//			pkgIndex = i
//			break
//		}
//	}
//	if pkgIndex == -1 {
//		return dir
//	}
//	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
//}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// NewConfig creates a config with ebpf-related settings
func NewConfig() *Config {
	cfg := aconfig.Datadog
	//cwd, err := curDir()
	//if err != nil {
	//	// default to repo structure and hope we are running from the root
	//	cwd = "pkg/ebpf"
	//}
	//bpfdir := filepath.Join(cwd, "bytecode/build")

	return &Config{
		BPFDebug:                 cfg.GetBool(key(spNS, "bpf_debug")),
		BPFDir:                   cfg.GetString(key(spNS, "bpf_dir")),
		ExcludedBPFLinuxVersions: cfg.GetStringSlice(key(spNS, "excluded_linux_versions")),
		EnableTracepoints:        cfg.GetBool(key(spNS, "enable_tracepoints")),
		ProcRoot:                 util.GetProcRoot(),

		EnableRuntimeCompiler:    cfg.GetBool(key(spNS, "enable_runtime_compiler")),
		RuntimeCompilerOutputDir: cfg.GetString(key(spNS, "runtime_compiler_output_dir")),
		KernelHeadersDirs:        cfg.GetStringSlice(key(spNS, "kernel_header_dirs")),
		AllowPrecompiledFallback: true,
	}
}
