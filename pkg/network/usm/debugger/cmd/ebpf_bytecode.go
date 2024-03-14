//go:build linux_bpf
// +build linux_bpf

package main

import (
	_ "embed"
	"os"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The code beloe is essentially responsible for embedding the CO-RE artifacts
// during compilation time and writing them to a temporary folder during
// runtime, so they can be loaded by the `usm.Monitor` as regular compilation
// assets.

//go:embed usm-debug.o
var usmProgram []byte

//go:embed http-debug.o
var httpProgram []byte

//go:embed shared-libraries-debug.o
var sharedLibrariesProgram []byte

func setupBytecode() func() {
	type program struct {
		filePath string
		bytecode []byte
	}

	var (
		bytecodeDir = os.TempDir()
		coreDir     = path.Join(bytecodeDir, "co-re")
	)

	os.Setenv("DD_SYSTEM_PROBE_BPF_DIR", bytecodeDir)
	err := os.MkdirAll(coreDir, os.ModePerm)
	checkError(err)

	programs := []program{
		{path.Join(coreDir, "usm-debug.o"), usmProgram},
		{path.Join(coreDir, "http-debug.o"), httpProgram},
		{path.Join(coreDir, "shared-libraries-debug.o"), sharedLibrariesProgram},
	}

	for _, p := range programs {
		f, err := os.Create(p.filePath)
		checkError(err)
		_, err = f.Write(p.bytecode)
		checkError(err)
		log.Debugf("writing ebpf bytecode to %s", p.filePath)
	}

	return func() {
		for _, p := range programs {
			os.Remove(p.filePath)
		}
	}
}
