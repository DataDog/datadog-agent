// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"errors"
	"os/exec"
	"sync"
	"testing"

	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type FakeEventConsumer struct {
	sync.RWMutex
	exec int
	fork int
	exit int
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

func (fc *FakeEventConsumer) HandleEvent(event *model.Event) {
	fc.Lock()
	defer fc.Unlock()

	switch event.GetEventType() {
	case model.ExecEventType:
		fc.exec++
	case model.ForkEventType:
		fc.fork++
	case model.ExitEventType:
		fc.exit++
	}
}

func TestEventMonitor(t *testing.T) {
	var fc *FakeEventConsumer
	test, err := newTestModule(t, nil, nil, testOpts{
		disableRuntimeSecurity: true,
		preStartCallback: func(test *testModule) {
			fc = NewFakeEventConsumer(test.eventMonitor)
			test.eventMonitor.RegisterEventConsumer(fc)
		},
	})
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
		_ = cmd.Run()

		err := retry.Do(func() error {
			if forkCount+1 <= fc.GetForkCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200), retry.Attempts(10))
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
		}, retry.Delay(200), retry.Attempts(10))
		assert.Nil(t, err)
	})
}
