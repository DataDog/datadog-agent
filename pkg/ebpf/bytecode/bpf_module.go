// +build linux_bpf

package bytecode

import (
	"bytes"
	"fmt"
)

// ReadBPFModule from the asset file
func ReadBPFModule(debug bool) (*bytes.Reader, error) {
	file := "pkg/ebpf/c/tracer-ebpf.o"
	if debug {
		file = "pkg/ebpf/c/tracer-ebpf-debug.o"
	}

	buf, err := Asset(file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return bytes.NewReader(buf), nil
}
