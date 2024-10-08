// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
)

type EbpfProgramSuite struct {
	suite.Suite
}

func TestEbpfProgram(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		if !IsSupported(ebpf.NewConfig()) {
			t.Skip("shared-libraries monitoring is not supported on this configuration")
		}

		suite.Run(t, new(EbpfProgramSuite))
	})
}

func (s *EbpfProgramSuite) TestCanInstantiateMultipleTimes() {
	t := s.T()
	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(prog.Stop)

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto))
	prog.Stop()

	prog2 := GetEBPFProgram(cfg)
	require.NotNil(t, prog2)

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto))
	t.Cleanup(prog2.Stop)
}

func (s *EbpfProgramSuite) TestProgramReceivesEventsWithSingleLibset() {
	t := s.T()
	fooPath1, _ := createTempTestFile(t, "foo-libssl.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(prog.Stop)

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto))

	var receivedEvent *LibPath
	cb := func(path LibPath) {
		lp := ToString(&path)
		if strings.Contains(lp, "foo-libssl.so") {
			receivedEvent = &path
		}
	}

	unsub, err := prog.Subscribe(cb, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsub)

	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)
	require.NotNil(t, command1.Process)
	t.Cleanup(func() {
		if command1 != nil && command1.Process != nil {
			command1.Process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		return receivedEvent != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPath1, ToString(receivedEvent))
	require.Equal(t, command1.Process.Pid, int(receivedEvent.Pid))
}

func (s *EbpfProgramSuite) TestSingleProgramReceivesMultipleLibsetEvents() {
	t := s.T()
	fooPathSsl, _ := createTempTestFile(t, "foo-libssl.so")
	fooPathCuda, _ := createTempTestFile(t, "foo-libcudart.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(prog.Stop)

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto, LibsetGPU))

	var receivedEventSsl, receivedEventCuda *LibPath
	cbSsl := func(path LibPath) {
		receivedEventSsl = &path
	}
	cbCuda := func(path LibPath) {
		receivedEventCuda = &path
	}

	unsubSsl, err := prog.Subscribe(cbSsl, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsubSsl)

	unsubCuda, err := prog.Subscribe(cbCuda, LibsetGPU)
	require.NoError(t, err)
	t.Cleanup(unsubCuda)

	commandSsl, err := fileopener.OpenFromAnotherProcess(t, fooPathSsl)
	require.NoError(t, err)
	require.NotNil(t, commandSsl.Process)
	t.Cleanup(func() {
		if commandSsl != nil && commandSsl.Process != nil {
			commandSsl.Process.Kill()
		}
	})

	commandCuda, err := fileopener.OpenFromAnotherProcess(t, fooPathCuda)
	require.NoError(t, err)
	require.NotNil(t, commandCuda.Process)
	t.Cleanup(func() {
		if commandCuda != nil && commandCuda.Process != nil {
			commandCuda.Process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		return receivedEventSsl != nil && receivedEventCuda != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPathSsl, ToString(receivedEventSsl))
	require.Equal(t, commandSsl.Process.Pid, int(receivedEventSsl.Pid))

	require.Equal(t, fooPathCuda, ToString(receivedEventCuda))
	require.Equal(t, commandCuda.Process.Pid, int(receivedEventCuda.Pid))
}

func (s *EbpfProgramSuite) TestMultpleProgramsReceiveMultipleLibsetEvents() {
	t := s.T()
	fooPathSsl, _ := createTempTestFile(t, "foo-libssl.so")
	fooPathCuda, _ := createTempTestFile(t, "foo-libcudart.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	progSsl := GetEBPFProgram(cfg)
	require.NotNil(t, progSsl)
	t.Cleanup(progSsl.Stop)

	require.NoError(t, progSsl.InitWithLibsets(LibsetCrypto))

	var receivedEventSsl *LibPath
	cbSsl := func(path LibPath) {
		receivedEventSsl = &path
	}

	unsubSsl, err := progSsl.Subscribe(cbSsl, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsubSsl)

	progCuda := GetEBPFProgram(cfg)
	require.NotNil(t, progCuda)
	t.Cleanup(progCuda.Stop)

	require.NoError(t, progCuda.InitWithLibsets(LibsetGPU))

	var receivedEventCuda *LibPath
	cbCuda := func(path LibPath) {
		receivedEventCuda = &path
	}

	unsubCuda, err := progCuda.Subscribe(cbCuda, LibsetGPU)
	require.NoError(t, err)
	t.Cleanup(unsubCuda)

	commandSsl, err := fileopener.OpenFromAnotherProcess(t, fooPathSsl)
	require.NoError(t, err)
	require.NotNil(t, commandSsl.Process)
	t.Cleanup(func() {
		if commandSsl != nil && commandSsl.Process != nil {
			commandSsl.Process.Kill()
		}
	})

	commandCuda, err := fileopener.OpenFromAnotherProcess(t, fooPathCuda)
	require.NoError(t, err)
	require.NotNil(t, commandCuda.Process)
	t.Cleanup(func() {
		if commandCuda != nil && commandCuda.Process != nil {
			commandCuda.Process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		return receivedEventSsl != nil && receivedEventCuda != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPathSsl, ToString(receivedEventSsl))
	require.Equal(t, commandSsl.Process.Pid, int(receivedEventSsl.Pid))

	require.Equal(t, fooPathCuda, ToString(receivedEventCuda))
	require.Equal(t, commandCuda.Process.Pid, int(receivedEventCuda.Pid))
}
