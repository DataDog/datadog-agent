// +build linux_bpf

package ebpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// ReadBPFModule from the asset file
func ReadBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "tracer.o"
	if debug {
		file = "tracer-debug.o"
	}

	ebpfReader, err := bytecode.GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return ebpfReader, nil
}

// ReadHTTPModule from the asset file
func ReadHTTPModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "http.o"
	if debug {
		file = "http-debug.o"
	}

	ebpfReader, err := bytecode.GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return ebpfReader, nil
}

// ReadOffsetBPFModule from the asset file
func ReadOffsetBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "offset-guess.o"
	if debug {
		file = "offset-guess-debug.o"
	}

	ebpfReader, err := bytecode.GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return ebpfReader, nil
}
