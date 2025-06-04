// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"bytes"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/codegen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/sm"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpfruntime "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
)

//go:generate $GOPATH/bin/include_headers pkg/dyninst/ebpf/event.c pkg/ebpf/bytecode/build/runtime/dyninst_event.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/dyninst_event.c pkg/ebpf/bytecode/runtime/dyninst_event.go runtime

func getCFlags(config *ddebpf.Config) []string {
	cflags := []string{
		"-g",
		"-Wno-unused-variable",
		"-Wno-unused-function",
		"-DDYNINST_GENERATED_CODE",
	}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

// CompileBPFProgram compiles the eBPF program.
func CompileBPFProgram(program ir.Program, extraCodeSink io.Writer) (ebpfruntime.CompiledOutput, error) {
	generatedCode := bytes.NewBuffer(nil)
	smProgram, err := sm.GenerateProgram(program)
	if err != nil {
		return nil, err
	}
	err = codegen.GenerateCCode(smProgram, generatedCode)
	if err != nil {
		return nil, err
	}

	injector := func(in io.Reader, out io.Writer) error {
		source, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		t, err := template.New("generated_code").Parse(string(source))
		if err != nil {
			return err
		}
		if extraCodeSink != nil {
			out = io.MultiWriter(out, extraCodeSink)
		}
		return t.Execute(out, generatedCode.String())
	}

	cfg := ddebpf.NewConfig()
	opts := ebpfruntime.CompileOptions{
		AdditionalFlags:  getCFlags(cfg),
		ModifyCallback:   injector,
		UseKernelHeaders: true,
	}
	return ebpfruntime.Dyninstevent.CompileWithOptions(cfg, opts)
}
