// +build linux_bpf

package bytecode

import (
	"bytes"
	"fmt"

	bpflib "github.com/iovisor/gobpf/elf"
)

// ReadBPFModule from the asset file
func ReadBPFModule(debug bool) (*bpflib.Module, error) {
	file := "tracer-ebpf.o"
	if debug {
		file = "tracer-ebpf-debug.o"
	}

	buf, err := Asset(file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	m := bpflib.NewModuleFromReader(bytes.NewReader(buf))
	if m == nil {
		return nil, fmt.Errorf("BPF not supported")
	}
	return m, nil
}
