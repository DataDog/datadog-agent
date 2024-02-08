// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/require"
)

func TestKprobeHelperProbe(t *testing.T) {
	err := rlimit.RemoveMemlock()
	require.NoError(t, err)

	var requiredFuncs = []asm.BuiltinFunc{
		asm.FnMapLookupElem,
		asm.FnMapUpdateElem,
		asm.FnMapDeleteElem,
		asm.FnPerfEventOutput,
		asm.FnPerfEventRead,
	}
	for _, rf := range requiredFuncs {
		if err := features.HaveProgramHelper(ebpf.Kprobe, rf); err != nil {
			if errors.Is(err, ebpf.ErrNotSupported) {
				t.Errorf("%s unsupported", rf.String())
			} else {
				t.Errorf("error checking for ebpf helper %s support: %s", rf.String(), err)
			}
		}
	}
}
