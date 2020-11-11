// +build linux_bpf

package compiler

/*
#cgo LDFLAGS: -lclangCodeGen -lclangFrontend -lclangSerialization -lclangDriver -lclangParse -lclangSema -lclangAnalysis -lclangASTMatchers -lclangRewrite -lclangEdit -lclangAST -lclangLex -lclangBasic
#cgo LDFLAGS: -L/opt/datadog-agent/embedded/lib
#cgo LDFLAGS: -lLLVMXRay -lLLVMWindowsManifest -lLLVMTableGen -lLLVMSymbolize -lLLVMDebugInfoPDB -lLLVMOrcJIT -lLLVMOrcError -lLLVMJITLink -lLLVMObjectYAML -lLLVMMIRParser -lLLVMMCJIT -lLLVMMCA -lLLVMLTO -lLLVMPasses -lLLVMCoroutines -lLLVMObjCARCOpts -lLLVMipo -lLLVMInstrumentation -lLLVMVectorize -lLLVMLinker -lLLVMIRReader -lLLVMAsmParser -lLLVMFrontendOpenMP -lLLVMExtensions -lLLVMLineEditor -lLLVMLibDriver -lLLVMGlobalISel -lLLVMFuzzMutate -lLLVMInterpreter -lLLVMExecutionEngine -lLLVMRuntimeDyld -lLLVMDWARFLinker -lLLVMDlltoolDriver -lLLVMOption -lLLVMDebugInfoGSYM -lLLVMCoverage -lLLVMCFGuard -lLLVMBPFDisassembler -lLLVMMCDisassembler -lLLVMBPFCodeGen -lLLVMSelectionDAG -lLLVMAsmPrinter -lLLVMDebugInfoDWARF -lLLVMCodeGen -lLLVMTarget -lLLVMScalarOpts -lLLVMInstCombine -lLLVMAggressiveInstCombine -lLLVMTransformUtils -lLLVMBitWriter -lLLVMAnalysis -lLLVMProfileData -lLLVMObject -lLLVMTextAPI -lLLVMBitReader -lLLVMCore -lLLVMRemarks -lLLVMBitstreamReader -lLLVMBPFAsmParser -lLLVMMCParser -lLLVMBPFDesc -lLLVMMC -lLLVMDebugInfoCodeView -lLLVMDebugInfoMSF -lLLVMBinaryFormat -lLLVMBPFInfo -lLLVMSupport -lLLVMDemangle
#cgo LDFLAGS: -lz -ldl -ltinfo
#cgo CPPFLAGS: -I/usr/include -I/opt/datadog-agent/embedded/include -D_GNU_SOURCE -D_DEBUG -D__STDC_CONSTANT_MACROS -D__STDC_FORMAT_MACROS -D__STDC_LIMIT_MACROS -DLLVM_MAJOR_VERSION=11

#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

type EBPFCompiler struct {
	compiler *C.struct_bpf_compiler

	verbose       bool
	defaultCflags []string
}

func (e *EBPFCompiler) CompileToObjectFile(inputFile, outputFile string, cflags []string) error {
	inputC := C.CString(inputFile)
	defer C.free(unsafe.Pointer(inputC))

	outputC := C.CString(outputFile)
	defer C.free(unsafe.Pointer(outputC))

	cflagsC := make([]*C.char, len(e.defaultCflags)+len(cflags)+1)
	for i, cflag := range e.defaultCflags {
		cflagsC[i] = C.CString(cflag)
	}
	for i, cflag := range cflags {
		cflagsC[i] = C.CString(cflag)
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

	if err := C.bpf_compile_to_object_file(e.compiler, inputC, outputC, (**C.char)(&cflagsC[0]), verboseC); err != 0 {
		errs := C.GoString(C.bpf_compiler_get_errors(e.compiler))
		return errors.New(errs)
	}

	return nil
}

func (e *EBPFCompiler) Close() {
	runtime.SetFinalizer(e, nil)
	C.delete_bpf_compiler(e.compiler)
	e.compiler = nil
}

func NewEBPFCompiler(verbose bool) *EBPFCompiler {
	ebpfCompiler := &EBPFCompiler{
		compiler: C.new_bpf_compiler(),
		verbose:  verbose,
	}

	runtime.SetFinalizer(ebpfCompiler, func(e *EBPFCompiler) {
		e.Close()
	})

	return ebpfCompiler
}
