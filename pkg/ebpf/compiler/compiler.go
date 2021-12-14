// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package compiler

/*
#cgo LDFLAGS: -lclangCodeGen -lclangFrontend -lclangSerialization -lclangDriver -lclangParse -lclangSema -lclangAnalysis -lclangASTMatchers -lclangRewrite -lclangEdit -lclangAST -lclangLex -lclangBasic
#cgo LDFLAGS: -L/opt/datadog-agent/embedded/lib
#cgo LDFLAGS: -lLLVMXRay -lLLVMWindowsManifest -lLLVMTableGen -lLLVMSymbolize -lLLVMDebugInfoPDB -lLLVMOrcJIT -lLLVMOrcError -lLLVMJITLink -lLLVMObjectYAML -lLLVMMIRParser -lLLVMMCJIT -lLLVMMCA -lLLVMLTO -lLLVMPasses -lLLVMCoroutines -lLLVMObjCARCOpts -lLLVMipo -lLLVMInstrumentation -lLLVMVectorize -lLLVMLinker -lLLVMIRReader -lLLVMAsmParser -lLLVMFrontendOpenMP -lLLVMExtensions -lLLVMLineEditor -lLLVMLibDriver -lLLVMGlobalISel -lLLVMFuzzMutate -lLLVMInterpreter -lLLVMExecutionEngine -lLLVMRuntimeDyld -lLLVMDWARFLinker -lLLVMDlltoolDriver -lLLVMOption -lLLVMDebugInfoGSYM -lLLVMCoverage -lLLVMCFGuard -lLLVMBPFDisassembler -lLLVMMCDisassembler -lLLVMBPFCodeGen -lLLVMSelectionDAG -lLLVMAsmPrinter -lLLVMDebugInfoDWARF -lLLVMCodeGen -lLLVMTarget -lLLVMScalarOpts -lLLVMInstCombine -lLLVMAggressiveInstCombine -lLLVMTransformUtils -lLLVMBitWriter -lLLVMAnalysis -lLLVMProfileData -lLLVMObject -lLLVMTextAPI -lLLVMBitReader -lLLVMCore -lLLVMRemarks -lLLVMBitstreamReader -lLLVMBPFAsmParser -lLLVMMCParser -lLLVMBPFDesc -lLLVMMC -lLLVMDebugInfoCodeView -lLLVMDebugInfoMSF -lLLVMBinaryFormat -lLLVMBPFInfo -lLLVMSupport -lLLVMDemangle
#cgo LDFLAGS: -lz -ldl -lm -lrt -static-libstdc++
#cgo LDFLAGS: -Wl,--wrap=exp -Wl,--wrap=log -Wl,--wrap=pow -Wl,--wrap=log2 -Wl,--wrap=log2f
#cgo CXXFLAGS: -I/opt/datadog-agent/embedded/include -std=c++14 -fno-exceptions -fno-rtti -D_GNU_SOURCE -D__STDC_CONSTANT_MACROS -D__STDC_FORMAT_MACROS -D__STDC_LIMIT_MACROS -DLLVM_MAJOR_VERSION=11
#cgo CPPFLAGS: -I/opt/datadog-agent/embedded/include -D_GNU_SOURCE -D_DEBUG -D__STDC_CONSTANT_MACROS -D__STDC_FORMAT_MACROS -D__STDC_LIMIT_MACROS -DLLVM_MAJOR_VERSION=11

#include <stdlib.h>
#include "wrapper.h"
#include "shim.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"runtime"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type EBPFCompiler struct {
	compiler *C.struct_bpf_compiler

	verbose      bool
	kernelCflags []string
}

func (e *EBPFCompiler) CompileFileToObjectFile(inputFile, outputFile string, cflags []string) error {
	inputC := C.CString(inputFile)
	defer C.free(unsafe.Pointer(inputC))

	return e.compile(inputC, outputFile, cflags, false)
}

func (e *EBPFCompiler) CompileToObjectFile(in io.Reader, outputFile string, cflags []string) error {
	inputBuf, err := ioutil.ReadAll(in)
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}
	inputC := (*C.char)(unsafe.Pointer(&inputBuf[0]))

	return e.compile(inputC, outputFile, cflags, true)
}

func (e *EBPFCompiler) compile(inputC *C.char, outputFile string, cflags []string, inMemory bool) error {
	outputC := C.CString(outputFile)
	defer C.free(unsafe.Pointer(outputC))

	cflagsC := make([]*C.char, len(e.kernelCflags)+len(cflags)+1)
	for i, cflag := range e.kernelCflags {
		cflagsC[i] = C.CString(cflag)
	}
	for i, cflag := range cflags {
		cflagsC[len(e.kernelCflags)+i] = C.CString(cflag)
	}
	cflagsC[len(cflagsC)-1] = nil

	defer func() {
		for _, cflag := range cflagsC {
			if cflag != nil {
				C.free(unsafe.Pointer(cflag))
			}
		}
	}()

	verboseC := C.char(0)
	if e.verbose {
		verboseC = 1
	}

	inMemoryC := C.char(0)
	if inMemory {
		inMemoryC = 1
	}

	if err := C.bpf_compile_to_object_file(e.compiler, inputC, outputC, (**C.char)(&cflagsC[0]), verboseC, inMemoryC); err != 0 {
		return fmt.Errorf("error compiling: %s", e.getErrors())
	}
	return nil
}

func (e *EBPFCompiler) getErrors() error {
	if e.compiler == nil {
		return nil
	}
	if errs := C.GoString(C.bpf_compiler_get_errors(e.compiler)); errs != "" {
		return errors.New(errs)
	}
	return nil
}

func (e *EBPFCompiler) Close() {
	runtime.SetFinalizer(e, nil)
	C.delete_bpf_compiler(e.compiler)
	e.compiler = nil
}

func NewEBPFCompiler(headerDirs []string, verbose bool) (*EBPFCompiler, error) {
	ebpfCompiler := &EBPFCompiler{
		compiler: C.new_bpf_compiler(),
		verbose:  verbose,
	}
	if err := ebpfCompiler.getErrors(); err != nil {
		ebpfCompiler.Close()
		return nil, err
	}

	runtime.SetFinalizer(ebpfCompiler, func(e *EBPFCompiler) {
		e.Close()
	})

	if len(headerDirs) == 0 {
		ebpfCompiler.Close()
		return nil, fmt.Errorf("unable to find kernel headers")
	}

	arch := kernel.Arch()
	if arch == "" {
		return nil, fmt.Errorf("unable to get kernel arch for %s", runtime.GOARCH)
	}

	var cflags []string
	for _, d := range headerDirs {
		cflags = append(cflags,
			fmt.Sprintf("-isystem%s/arch/%s/include", d, arch),
			fmt.Sprintf("-isystem%s/arch/%s/include/generated", d, arch),
			fmt.Sprintf("-isystem%s/include", d),
			fmt.Sprintf("-isystem%s/arch/%s/include/uapi", d, arch),
			fmt.Sprintf("-isystem%s/arch/%s/include/generated/uapi", d, arch),
			fmt.Sprintf("-isystem%s/include/uapi", d),
			fmt.Sprintf("-isystem%s/include/generated/uapi", d),
		)
	}
	ebpfCompiler.kernelCflags = cflags

	return ebpfCompiler, nil
}
