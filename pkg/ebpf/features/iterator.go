// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package features

import (
	"errors"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// HaveIteratorType returns whether the named bpf iterator programs are supported
var HaveIteratorType = funcs.MemoizeArgNoError(func(iteratorType string) error {
	if err := features.HaveProgramType(ebpf.Tracing); err != nil {
		return err
	}

	spec := &ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: ebpf.AttachTraceIter,
		AttachTo:   iteratorType,
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
		License: "GPL",
	}

	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		switch {
		// EINVAL occurs when attempting to create a program with an unknown type.
		// E2BIG occurs when ProgLoadAttr contains non-zero bytes past the end
		// of the struct known by the running kernel, meaning the kernel is too old
		// to support the given prog type.
		case errors.Is(err, unix.EINVAL), errors.Is(err, unix.E2BIG):
			err = ebpf.ErrNotSupported
		}
		return err
	}
	defer prog.Close()

	l, err := link.AttachIter(link.IterOptions{
		Program: prog,
	})
	if err != nil {
		return err
	}
	defer l.Close()

	return nil
})
