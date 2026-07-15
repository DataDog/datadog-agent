// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package features

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// HaveHelperInFentry returns whether the helper and attach type are supported together.
var HaveHelperInFentry = funcs.MemoizeArgNoError(func(helper asm.BuiltinFunc) error {
	if err := features.HaveProgramType(ebpf.Tracing); err != nil {
		return err
	}

	spec := &ebpf.ProgramSpec{
		AttachType: ebpf.AttachTraceFEntry,
		AttachTo:   "tcp_connect",
		Type:       ebpf.Tracing,
		Instructions: asm.Instructions{
			helper.Call(),
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
		License: "GPL",
	}

	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogLevel: 1,
	})
	if err == nil {
		prog.Close()
	}

	var verr *ebpf.VerifierError
	if !errors.As(err, &verr) {
		return err
	}

	helperTag := fmt.Sprintf("#%d", helper)

	switch {
	// EACCES occurs when attempting to create a program probe with a helper
	// while the register args when calling this helper aren't set up properly.
	// We interpret this as the helper being available, because the verifier
	// returns EINVAL if the helper is not supported by the running kernel.
	case errors.Is(err, unix.EACCES):
		err = nil

	// EINVAL occurs when attempting to create a program with an unknown helper.
	case errors.Is(err, unix.EINVAL):
		// https://github.com/torvalds/linux/blob/09a0fa92e5b45e99cf435b2fbf5ebcf889cf8780/kernel/bpf/verifier.c#L10663
		if logContainsAll(verr.Log, "invalid func", helperTag) {
			return ebpf.ErrNotSupported
		}

		// https://github.com/torvalds/linux/blob/09a0fa92e5b45e99cf435b2fbf5ebcf889cf8780/kernel/bpf/verifier.c#L10668
		wrongProgramType := logContainsAll(verr.Log, "program of this type cannot use helper", helperTag)
		// https://github.com/torvalds/linux/blob/59b418c7063d30e0a3e1f592d47df096db83185c/kernel/bpf/verifier.c#L10204
		// 4.9 doesn't include # in verifier output.
		wrongProgramType = wrongProgramType || logContainsAll(verr.Log, "unknown func")
		if wrongProgramType {
			return fmt.Errorf("program of this type cannot use helper: %w", ebpf.ErrNotSupported)
		}
	}

	return err
})

func logContainsAll(log []string, needles ...string) bool {
	first := max(len(log)-5, 0) // Check last 5 lines.
	return slices.ContainsFunc(log[first:], func(line string) bool {
		for _, needle := range needles {
			if !strings.Contains(line, needle) {
				return false
			}
		}
		return true
	})
}
