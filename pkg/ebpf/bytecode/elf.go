// +build linux_bpf

package bytecode

import (
	"fmt"
	"os"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	gore "github.com/lebauce/go-re"
)

var defaultCompilationFlags = []string{
	"-DRUNTIME_COMPILATION=1",
	"-DKBUILD_MODNAME=\"foo\"",
	"-D__KERNEL__",
	"-DCONFIG_64BIT",
	"-D__BPF_TRACING__",
	"-Wno-unused-value",
	"-Wno-pointer-sign",
	"-Wno-compare-distinct-pointer-types",
	"-Wunused",
	"-Wall",
	"-Werror",
}

// Options consolidates all configuration params associated to the eBPF byte code management
type Options struct {
	BPFDir               string
	Debug                bool
	EnableIPv6           bool
	OffsetGuessThreshold uint64
}

// GetNetworkTracerELF obtains the eBPF bytecode used by the Network Tracer.
// First, it attempts to compile the eBPF bytecode on the fly, using the host kernel
// headers and the linked compiler. If this process fails for some reason
// (eg. system headers not available), we fall back to the pre-compiled ELF file
// that relies on offset guessing.
func GetNetworkTracerELF(opts Options) (AssetReader, []manager.ConstantEditor, error) {
	asset, err := compileEBPFByteCode(opts)
	if err != nil {
		log.Errorf("error compiling eBPF bytecode: %s. falling back to pre-compiled ELF file")
		return getPrecompiledELF(opts)
	}

	return asset, nil, nil
}

func compileEBPFByteCode(opts Options) (AssetReader, error) {
	compiler := gore.NewEBPFCompiler(true)
	defer compiler.Close()

	cflags, err := getCFLAGS(opts)
	if err != nil {
		return nil, fmt.Errorf("error generating compilation flags: %s")
	}

	src := path.Join(opts.BPFDir, "tracer-ebpf.c")
	out := path.Join(os.TempDir(), "tracer-ebpf.o")
	log.Debugf("compiling eBPF code. flags=%q src=%s out=%s", cflags, src, out)
	err = compiler.CompileToObjectFile(src, out, cflags)
	if err != nil {
		return nil, fmt.Errorf("failed to compile eBPF bytecode: %s", err)
	}

	return os.Open(out)
}

func getPrecompiledELF(opts Options) (AssetReader, []manager.ConstantEditor, error) {
	constants, err := GuessOffsets(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("could not obtain offsets: %s", err)
	}

	elf, err := ReadBPFModule(opts.BPFDir, opts.Debug)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read bpf module: %s", err)
	}

	return elf, constants, nil
}

func getCFLAGS(opts Options) ([]string, error) {
	// Retrieve default flags
	flags := make([]string, len(defaultCompilationFlags))
	copy(flags, defaultCompilationFlags)

	// Include asm_goto_workaround.h
	flags = append(
		flags,
		fmt.Sprintf("-include%s", path.Join(opts.BPFDir, "asm_goto_workaround.h")),
	)

	// Configure include path for kernel headers
	includePaths, err := getKernelIncludePaths()
	if err != nil {
		return nil, fmt.Errorf("error retrieving kernel headers: %s", err)
	}

	for _, path := range includePaths {
		flags = append(flags, fmt.Sprintf("-isystem%s", path))
	}

	return flags, nil
}
