// +build linux_bpf

package bytecode

import (
	"fmt"

	bpflib "github.com/iovisor/gobpf/elf"
)

// ReadBPFModule from the asset file
func ReadBPFModule(bpfDir string, debug bool) (*bpflib.Module, error) {
	file := "pkg/ebpf/c/tracer-ebpf.o"
	if debug {
		file = "pkg/ebpf/c/tracer-ebpf-debug.o"
	}

	ebpfReader, err := GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	m := bpflib.NewModuleFromReader(ebpfReader)
	if m == nil {
		return nil, fmt.Errorf("BPF not supported")
	}
	return m, nil
}
