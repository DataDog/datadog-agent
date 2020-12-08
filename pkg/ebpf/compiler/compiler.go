// +build linux_bpf

package compiler

/*
#cgo LDFLAGS: -lclangCodeGen -lclangFrontend -lclangSerialization -lclangDriver -lclangParse -lclangSema -lclangAnalysis -lclangASTMatchers -lclangRewrite -lclangEdit -lclangAST -lclangLex -lclangBasic
#cgo LDFLAGS: -L/opt/datadog-agent/embedded/lib
#cgo LDFLAGS: -lLLVMXRay -lLLVMWindowsManifest -lLLVMTableGen -lLLVMSymbolize -lLLVMDebugInfoPDB -lLLVMOrcJIT -lLLVMOrcError -lLLVMJITLink -lLLVMObjectYAML -lLLVMMIRParser -lLLVMMCJIT -lLLVMMCA -lLLVMLTO -lLLVMPasses -lLLVMCoroutines -lLLVMObjCARCOpts -lLLVMipo -lLLVMInstrumentation -lLLVMVectorize -lLLVMLinker -lLLVMIRReader -lLLVMAsmParser -lLLVMFrontendOpenMP -lLLVMExtensions -lLLVMLineEditor -lLLVMLibDriver -lLLVMGlobalISel -lLLVMFuzzMutate -lLLVMInterpreter -lLLVMExecutionEngine -lLLVMRuntimeDyld -lLLVMDWARFLinker -lLLVMDlltoolDriver -lLLVMOption -lLLVMDebugInfoGSYM -lLLVMCoverage -lLLVMCFGuard -lLLVMBPFDisassembler -lLLVMMCDisassembler -lLLVMBPFCodeGen -lLLVMSelectionDAG -lLLVMAsmPrinter -lLLVMDebugInfoDWARF -lLLVMCodeGen -lLLVMTarget -lLLVMScalarOpts -lLLVMInstCombine -lLLVMAggressiveInstCombine -lLLVMTransformUtils -lLLVMBitWriter -lLLVMAnalysis -lLLVMProfileData -lLLVMObject -lLLVMTextAPI -lLLVMBitReader -lLLVMCore -lLLVMRemarks -lLLVMBitstreamReader -lLLVMBPFAsmParser -lLLVMMCParser -lLLVMBPFDesc -lLLVMMC -lLLVMDebugInfoCodeView -lLLVMDebugInfoMSF -lLLVMBinaryFormat -lLLVMBPFInfo -lLLVMSupport -lLLVMDemangle
#cgo LDFLAGS: -lz -ldl -ltinfo -lm -lrt
#cgo CXXFLAGS: -I/opt/datadog-agent/embedded/include -std=c++14 -fno-exceptions -fno-rtti -D_GNU_SOURCE -D__STDC_CONSTANT_MACROS -D__STDC_FORMAT_MACROS -D__STDC_LIMIT_MACROS -DLLVM_MAJOR_VERSION=11
#cgo CPPFLAGS: -I/opt/datadog-agent/embedded/include -D_GNU_SOURCE -D_DEBUG -D__STDC_CONSTANT_MACROS -D__STDC_FORMAT_MACROS -D__STDC_LIMIT_MACROS -DLLVM_MAJOR_VERSION=11

#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
		cflagsC[len(e.defaultCflags)+i] = C.CString(cflag)
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

	cflags := []string{
		"-DCONFIG_64BIT",
		"-D__BPF_TRACING__",
		`-DKBUILD_MODNAME='"ddsysprobe"'`,
		"-Wno-unused-value",
		"-Wno-pointer-sign",
		"-Wno-compare-distinct-pointer-types",
		"-Wunused",
		"-Wall",
		"-Werror",
	}

	var err error
	var dirs []string
	if len(headerDirs) > 0 {
		for _, d := range headerDirs {
			err = kernel.ValidateHeaderDir(d)
			if err != nil {
				if os.IsNotExist(err) {
					// allow missing version.h errors
					continue
				}
				ebpfCompiler.Close()
				return nil, fmt.Errorf("error validating kernel header directories: %w", err)
			}
			// as long as one directory passes, use the entire set
			dirs = headerDirs
			break
		}
	} else {
		dirs, err = kernel.FindHeaderDirs()
		if err != nil {
			ebpfCompiler.Close()
			return nil, fmt.Errorf("unable to find kernel headers: %w", err)
		}
	}

	if len(dirs) == 0 {
		ebpfCompiler.Close()
		return nil, fmt.Errorf("unable to find kernel headers")
	}

	for _, d := range dirs {
		cflags = append(cflags,
			fmt.Sprintf("-isystem%s/arch/x86/include", d),
			fmt.Sprintf("-isystem%s/arch/x86/include/generated", d),
			fmt.Sprintf("-isystem%s/include", d),
			fmt.Sprintf("-isystem%s/arch/x86/include/uapi", d),
			fmt.Sprintf("-isystem%s/arch/x86/include/generated/uapi", d),
			fmt.Sprintf("-isystem%s/include/uapi", d),
			fmt.Sprintf("-isystem%s/include/generated/uapi", d),
		)
	}
	ebpfCompiler.defaultCflags = cflags

	return ebpfCompiler, nil
}
