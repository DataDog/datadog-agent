// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"testing"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

//go:generate $GOPATH/bin/include_headers pkg/ebpf/testdata/c/runtime/logdebug-test.c pkg/ebpf/bytecode/build/runtime/logdebug-test.c pkg/ebpf/c pkg/network/ebpf/c/runtime pkg/network/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/logdebug-test.c pkg/ebpf/bytecode/runtime/logdebug-test.go runtime

func TestPatchPrintkNewline(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		cfg := ddebpf.NewConfig()
		require.NotNil(t, cfg)

		buf, err := LogdebugTest.Compile(cfg, []string{"-g", "-DDEBUG=1"}, nil)
		require.NoError(t, err)
		defer buf.Close()

		idPair := manager.ProbeIdentificationPair{
			EBPFFuncName: "logdebugtest",
			UID:          "logdebugtest",
		}
		mgr := &manager.Manager{
			Probes: []*manager.Probe{
				{
					ProbeIdentificationPair: idPair,
					XDPAttachMode:           manager.XdpAttachModeSkb,
					IfIndex:                 1,
				},
			},
		}

		opts := manager.Options{
			RLimit: &unix.Rlimit{
				Cur: math.MaxUint64,
				Max: math.MaxUint64,
			},
			MapEditors: make(map[string]*ebpf.Map),
			ConstantEditors: []manager.ConstantEditor{
				ddebpf.GetPatchedPrintkEditor(),
			},
		}

		tp, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
		require.NoError(t, err)
		traceReader := bufio.NewReader(tp)
		t.Cleanup(func() { _ = tp.Close() })

		require.NoError(t, mgr.InitWithOptions(buf, opts))
		require.NoError(t, mgr.Start())
		t.Cleanup(func() { mgr.Stop(manager.CleanAll) })

		progs, ok, err := mgr.GetProgram(idPair)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, progs)
		require.NotEmpty(t, progs)
		prog := progs[0]

		input := make([]byte, 30) // Enough for the arguments
		ret, _, err := prog.Test(input)
		require.NoError(t, err)
		require.Equal(t, uint32(42), ret) // Check that the program did actually execute

		// The logdebug-test.c program outputs two lines: Hello, world! and Goodbye, world!
		// Check that those two are the lines included in the trace_pipe output, with no
		// additional lines or empty lines. We check with "contains" to avoid issues with
		// the variable output (PID, time, etc)
		line1, err := traceReader.ReadString('\n')
		fmt.Println(line1)
		require.NoError(t, err)
		require.Contains(t, line1, "Hello, world!")

		line2, err := traceReader.ReadString('\n')
		fmt.Println(line2)
		require.NoError(t, err)
		require.Contains(t, line2, "Goodbye, world!")
	})
}
