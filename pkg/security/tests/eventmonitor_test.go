// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type FakeEventConsumer struct {
	sync.RWMutex
	exec             int
	fork             int
	exit             int
	lastReceivedExec *FakeConsumerProcess
	lastReceivedFork *FakeConsumerProcess
	lastReceivedExit *FakeConsumerProcess
}

type FakeConsumerProcess struct {
	EventType      model.EventType
	Pid            uint32
	Envs           []string
	ContainerID    *intern.Value
	StartTime      int64
	Expiry         int64
	CollectionTime time.Time
	//Ppid           uint32
	//UID            uint32
	//GID            uint32
	//Username       string
	//Group          string
	//Exe            string
	//Cmdline        []string
	//ForkTime       time.Time
	//ExecTime       time.Time
	//ExitTime       time.Time
	//ExitCode       uint32
}

func NewFakeEventConsumer(em *eventmonitor.EventMonitor) *FakeEventConsumer {
	fc := &FakeEventConsumer{}

	_ = em.AddEventTypeHandler(model.ForkEventType, fc)
	_ = em.AddEventTypeHandler(model.ExecEventType, fc)
	_ = em.AddEventTypeHandler(model.ExitEventType, fc)

	return fc
}

func (fc *FakeEventConsumer) ID() string {
	return "FAKE"
}

func (fc *FakeEventConsumer) Start() error {
	return nil
}

func (fc *FakeEventConsumer) Stop() {
}

func (fc *FakeEventConsumer) GetForkCount() int {
	fc.RLock()
	defer fc.RUnlock()
	return fc.fork
}

func (fc *FakeEventConsumer) GetExitCount() int {
	fc.RLock()
	defer fc.RUnlock()
	return fc.exit
}

func (fc *FakeEventConsumer) GetExecCount() int {
	fc.RLock()
	defer fc.RUnlock()
	return fc.exec
}

func (fc *FakeEventConsumer) HandleEvent(incomingEvent any) {
	event, ok := incomingEvent.(*FakeConsumerProcess)
	if !ok {
		log.Error("Event is not a security model event")
		return
	}

	fc.Lock()
	defer fc.Unlock()

	switch event.EventType {
	case model.ExecEventType:
		fc.exec++
		fc.lastReceivedExec = event
	case model.ForkEventType:
		fc.fork++
		fc.lastReceivedFork = event
	case model.ExitEventType:
		fc.exit++
		fc.lastReceivedExit = event
	}
}

func (fc *FakeEventConsumer) Copy(ev *model.Event) any {
	var processStartTime time.Time
	if ev.GetEventType() == model.ExecEventType {
		processStartTime = ev.GetProcessExecTime()
	}
	if ev.GetEventType() == model.ForkEventType {
		processStartTime = ev.GetProcessForkTime()
	}

	return &FakeConsumerProcess{
		EventType:   ev.GetEventType(),
		Pid:         ev.GetProcessPid(),
		ContainerID: intern.GetByString(ev.GetContainerId()),
		StartTime:   processStartTime.UnixNano(),
		Envs:        ev.GetProcessEnvp(),
	}
}

func TestEventMonitor(t *testing.T) {
	var fc *FakeEventConsumer
	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		disableRuntimeSecurity: true,
		preStartCallback: func(test *testModule) {
			fc = NewFakeEventConsumer(test.eventMonitor)
			test.eventMonitor.RegisterEventConsumer(fc)
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fork", func(t *testing.T) {
		forkCount := fc.GetForkCount()
		cmd := exec.Command(syscallTester, "fork")
		cmd.Env = append(os.Environ(), "DD_SERVICE=myService", "DD_VERSION=0.1.0", "DD_ENV=myEnv", "EXTRAVAR=extra")
		_ = cmd.Run()

		err := retry.Do(func() error {
			if forkCount+1 <= fc.GetForkCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10))
		fmt.Printf("%+v\n", fc.lastReceivedFork)
		assert.Subset(t, fc.lastReceivedFork.Envs, []string{"DD_SERVICE=myService", "DD_VERSION=0.1.0", "DD_ENV=myEnv"})
		assert.Nil(t, err)
	})

	t.Run("exec-exit", func(t *testing.T) {
		execCount := fc.GetExecCount()
		exitCount := fc.GetExitCount()

		lsExecutable := which(t, "ls")
		cmd := exec.Command(lsExecutable, "-l")
		_ = cmd.Run()

		err := retry.Do(func() error {
			if execCount+1 <= fc.GetExecCount() && exitCount+1 <= fc.GetExitCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10))
		assert.Nil(t, err)
	})
}
