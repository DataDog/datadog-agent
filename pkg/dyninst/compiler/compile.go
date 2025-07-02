// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/codegen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/sm"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	ebpfruntime "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
)

// RingbufMapName is the name of the ringbuffer map that is used to collect
// probe output.
const RingbufMapName = "out_ringbuf"

//go:generate $GOPATH/bin/include_headers pkg/dyninst/ebpf/event.c pkg/ebpf/bytecode/build/runtime/dyninst_event.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/dyninst_event.c pkg/ebpf/bytecode/runtime/dyninst_event.go runtime

// CompiledBPF holds compiled object of the eBPF program and its metadata.
type CompiledBPF struct {
	Obj ebpfruntime.CompiledOutput

	// Program to attach and list of pcs to attach at, along with the cookie
	// that should be provided at that attach point.
	ProgramName  string
	Attachpoints []codegen.BPFAttachPoint
}

func compileBPFProgram(
	cfg *config, program *ir.Program, extraCodeSink io.Writer,
) (CompiledBPF, error) {
	generatedCode := bytes.NewBuffer(nil)
	smProgram, err := sm.GenerateProgram(program)
	if err != nil {
		return CompiledBPF{}, err
	}
	attachpoints, err := codegen.GenerateCCode(smProgram, generatedCode)
	if err != nil {
		return CompiledBPF{}, err
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

	opts := ebpfruntime.CompileOptions{
		AdditionalFlags:  getCFlags(cfg),
		ModifyCallback:   injector,
		UseKernelHeaders: true,
	}
	obj, err := ebpfruntime.Dyninstevent.CompileWithOptions(
		cfg.ebpfConfig, opts,
	)
	if err != nil {
		return CompiledBPF{}, err
	}

	return CompiledBPF{Obj: obj, ProgramName: "probe_run_with_cookie", Attachpoints: attachpoints}, nil
}

func getCFlags(cfg *config) []string {
	cflags := []string{
		"-g",
		"-Wno-unused-variable",
		"-Wno-unused-function",
		"-DDYNINST_GENERATED_CODE",
	}
	if cfg.ebpfConfig.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	if cfg.dyninstDebugEnabled {
		cflags = append(
			cflags,
			fmt.Sprintf("-DDYNINST_DEBUG=%d", cfg.dyninstDebugLevel),
		)
	}
	return cflags
}
