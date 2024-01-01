// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This is the number of events that are expected to be received by the event monitor at test module initialization, before any test commands have been run
const (
	testModuleInitialExecs = 34
	testModuleInitialForks = 0
	testModuleInitialExits = 0
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
	CollectionTime time.Time
	ExitTime       time.Time
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
	var exitTime time.Time
	if ev.GetEventType() == model.ExecEventType {
		processStartTime = ev.GetProcessExecTime()
	}
	if ev.GetEventType() == model.ForkEventType {
		processStartTime = ev.GetProcessForkTime()
	}
	if ev.GetEventType() == model.ExitEventType {
		exitTime = ev.GetProcessExitTime()
	}

	return &FakeConsumerProcess{
		EventType:   ev.GetEventType(),
		Pid:         ev.GetProcessPid(),
		ContainerID: intern.GetByString(ev.GetContainerId()),
		StartTime:   processStartTime.UnixNano(),
		ExitTime:    exitTime,
		Envs:        ev.GetProcessEnvp(),
	}
}

func (fc *FakeEventConsumer) Reset() {

}

func TestEventMonitor(t *testing.T) {
	var fc *FakeEventConsumer
	testModule, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		disableRuntimeSecurity: true,
		preStartCallback: func(test *testModule) {
			fc = NewFakeEventConsumer(test.eventMonitor)
			test.eventMonitor.RegisterEventConsumer(fc)
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer testModule.Close()

	syscallTester, err := loadSyscallTester(t, testModule, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	updatedExecs := testModuleInitialExecs
	updatedForks := testModuleInitialForks
	updatedExits := testModuleInitialExits

	envVars := []string{"DD_SERVICE=myService", "DD_VERSION=0.1.0", "DD_ENV=myEnv"}

	tests := []struct {
		name         string
		commandToRun *exec.Cmd
		check        func(c *assert.CollectT)
	}{
		{
			name:         "fork",
			commandToRun: exec.Command(syscallTester, "fork"),
			check: func(c *assert.CollectT) {
				assert.GreaterOrEqual(t, fc.GetExecCount(), updatedExecs)
				assert.GreaterOrEqual(t, fc.GetForkCount(), updatedForks)
				assert.GreaterOrEqual(t, fc.GetExitCount(), updatedExits)

				assert.Subset(t, fc.lastReceivedFork.Envs, envVars)
			},
		},
		{
			name:         "exec-exit",
			commandToRun: exec.Command(which(t, "ls"), "-l"),
			check: func(c *assert.CollectT) {
				assert.GreaterOrEqual(t, fc.GetExecCount(), updatedExecs)
				assert.GreaterOrEqual(t, fc.GetForkCount(), updatedForks)
				assert.GreaterOrEqual(t, fc.GetExitCount(), updatedExits)

				assert.Subset(t, fc.lastReceivedExec.Envs, envVars)
				assert.Subset(t, fc.lastReceivedExit.Envs, envVars)

				assert.Greater(t, fc.lastReceivedExit.ExitTime, time.Unix(fc.lastReceivedExec.StartTime, 0))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.commandToRun.Env = append(os.Environ(), "DD_SERVICE=myService", "DD_VERSION=0.1.0", "DD_ENV=myEnv", "EXTRAVAR=extra")
			_ = test.commandToRun.Run()

			// Running a command with the syscall tester creates more than 1 event per type, so this incrementation is just an estimate of the real count,
			// and all tests should use comparisons instead of equality
			updatedExecs++
			updatedForks++
			updatedExits++

			assert.EventuallyWithTf(t, test.check, 200*time.Millisecond*12, 200*time.Millisecond, "event monitor has not received an expected event yet")
		})
	}
}
