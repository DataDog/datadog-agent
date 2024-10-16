// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"strings"

	"sync"
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

	var eventMutex sync.Mutex
	var receivedEvent *LibPath
	cb := func(path LibPath) {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		lp := path.String()
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
		eventMutex.Lock()
		defer eventMutex.Unlock()
		return receivedEvent != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPath1, receivedEvent.String())
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

	var eventMutex sync.Mutex
	var receivedEventSsl, receivedEventCuda *LibPath
	cbSsl := func(path LibPath) {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		receivedEventSsl = &path
	}
	cbCuda := func(path LibPath) {
		eventMutex.Lock()
		defer eventMutex.Unlock()
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
		eventMutex.Lock()
		defer eventMutex.Unlock()
		return receivedEventSsl != nil && receivedEventCuda != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPathSsl, receivedEventSsl.String())
	require.Equal(t, commandSsl.Process.Pid, int(receivedEventSsl.Pid))

	require.Equal(t, fooPathCuda, receivedEventCuda.String())
	require.Equal(t, commandCuda.Process.Pid, int(receivedEventCuda.Pid))
}

func (s *EbpfProgramSuite) TestMultipleProgramsReceiveMultipleLibsetEvents() {
	t := s.T()
	fooPathSsl, _ := createTempTestFile(t, "foo-libssl.so")
	fooPathCuda, _ := createTempTestFile(t, "foo-libcudart.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	progSsl := GetEBPFProgram(cfg)
	require.NotNil(t, progSsl)
	t.Cleanup(progSsl.Stop)

	require.NoError(t, progSsl.InitWithLibsets(LibsetCrypto))

	// To ensure that we're not having data races in the test code
	var receivedEventMutex sync.Mutex

	var receivedEventSsl *LibPath
	cbSsl := func(path LibPath) {
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
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
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
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
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
		return receivedEventSsl != nil && receivedEventCuda != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPathSsl, receivedEventSsl.String())
	require.Equal(t, commandSsl.Process.Pid, int(receivedEventSsl.Pid))

	require.Equal(t, fooPathCuda, receivedEventCuda.String())
	require.Equal(t, commandCuda.Process.Pid, int(receivedEventCuda.Pid))
}
