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
	compiler := gore.NewEBPFCompiler(true)
	defer compiler.Close()

	var (
		in  = path.Join(opts.BPFDir, "tracer-ebpf.c")
		out = path.Join(os.TempDir(), "tracer-ebpf.o")
	)

	cflags := getCFLAGS(opts)
	log.Debugf("compiling eBPF code with the following flags: %q", cflags)
	err := compiler.CompileToObjectFile(in, out, cflags)
	if err != nil {
		log.Errorf("failed to compile eBPF bytecode: %s. falling back to pre-compiled bytecode.", err.Error())
		return getPrecompiledELF(opts)
	}

	log.Debugf("eBPF bytecode available in %s", out)
	f, err := os.Open(out)
	return f, nil, err
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

func getCFLAGS(opts Options) []string {
	// Retrieve default flags
	flags := make([]string, len(defaultCompilationFlags))
	copy(flags, defaultCompilationFlags)

	// Include asm_goto_workaround.h
	flags = append(
		flags,
		fmt.Sprintf("-include%s", path.Join(opts.BPFDir, "asm_goto_workaround.h")),
	)

	// Configure include path for kernel headers
	for _, path := range getKernelIncludePaths() {
		flags = append(flags, fmt.Sprintf("-isystem%s", path))
	}

	return flags
}
