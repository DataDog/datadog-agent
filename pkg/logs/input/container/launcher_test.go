// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/stretchr/testify/assert"
)

type mockLauncher struct {
	wg         sync.WaitGroup
	m          sync.Mutex
	retrier    retry.Retrier
	isAvalible bool
	startCount int
	stopCount  int
	attempt    uint
}

func newMockLauncher(isAvalible bool) *mockLauncher {
	l := &mockLauncher{isAvalible: isAvalible}
	l.retrier.SetupRetrier(&retry.Config{
		Name:          "testing",
		AttemptMethod: func() error { return nil },
		Strategy:      retry.JustTesting,
		RetryCount:    0,
		RetryDelay:    time.Millisecond,
	})
	return l
}

func newMockLauncherBecomesAvalible(avalibleAfterRetries uint) *mockLauncher {
	l := &mockLauncher{isAvalible: false}
	l.retrier.SetupRetrier(&retry.Config{
		Name: "testing",
		AttemptMethod: func() error {
			l.attempt++
			if l.attempt > avalibleAfterRetries {
				l.isAvalible = true
				l.wg.Done()
				return nil
			}
			return errors.New("")
		},
		Strategy:   retry.RetryCount,
		RetryCount: 100,
		RetryDelay: time.Millisecond,
	})
	return l
}

func (m *mockLauncher) IsAvalible() (bool, *retry.Retrier) {
	defer m.m.Unlock()
	m.m.Lock()
	m.retrier.TriggerRetry()
	return m.isAvalible, &m.retrier
}

func (m *mockLauncher) SetAvalible(avalible bool) {
	defer m.m.Unlock()
	m.m.Lock()
	m.isAvalible = avalible
}

func (m *mockLauncher) Start() {
	m.startCount++
	m.wg.Done()
}
func (m *mockLauncher) Stop() {
	m.stopCount++
}

func (m *mockLauncher) ToLaunchable() Launchable {
	return Launchable{
		IsAvailable: m.IsAvalible,
		Launcher: func() restart.Restartable {
			m.wg.Done()
			return m
		},
	}
}

func (m *mockLauncher) ToErrLaunchable() Launchable {
	return Launchable{
		IsAvailable: m.IsAvalible,
		Launcher: func() restart.Restartable {
			m.wg.Done()
			return nil
		},
	}
}

func TestSelectFirst(t *testing.T) {
	l1 := newMockLauncher(true)
	l2 := newMockLauncher(false)

	l1.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l1.wg.Wait()
	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestSelectSecond(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(true)

	l2.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l2.wg.Wait()
	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 1, l2.startCount)
}

func TestFailsThenSucceeds(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(false)

	l2.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	// let it run a few times
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 0, l2.startCount)

	l2.SetAvalible(true)
	l2.wg.Wait()

	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 1, l2.startCount)
}

func TestFailsThenSucceedsRetrier(t *testing.T) {
	l1 := newMockLauncherBecomesAvalible(3)
	l2 := newMockLauncher(false)

	l1.wg.Add(3)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l1.wg.Wait()

	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestAvalibleLauncherReturnsNil(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(true)

	l2.wg.Add(1)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToErrLaunchable()})
	l.Start()

	l2.wg.Wait()
	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
	l.Lock()
	_, ok := l.activeLauncher.(*noopLauncher)
	l.Unlock()
	assert.True(t, ok)
}

func TestRestartUsesPreviousLauncher(t *testing.T) {
	l1 := newMockLauncher(true)
	l2 := newMockLauncher(false)

	l1.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l1.wg.Wait()
	l.Stop()
	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 1, l1.stopCount)
	assert.Equal(t, 0, l2.startCount)
	assert.Equal(t, 0, l2.stopCount)

	l1.wg.Add(1)
	l.Start()
	l1.wg.Wait()

	assert.Equal(t, 2, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestRestartFindLauncherLater(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(false)

	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	// let it run a few times
	time.Sleep(10 * time.Millisecond)
	l.Stop()

	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 0, l1.stopCount)
	assert.Equal(t, 0, l2.startCount)
	assert.Equal(t, 0, l2.stopCount)

	l1.SetAvalible(true)

	l1.wg.Add(2)
	l.Start()
	l1.wg.Wait()

	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestRestartSameLauncher(t *testing.T) {
	l1 := newMockLauncher(true)
	l2 := newMockLauncher(false)

	l1.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	// let it run a few times
	time.Sleep(10 * time.Millisecond)
	l.Stop()
	l1.wg.Wait()

	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 1, l1.stopCount)
	assert.Equal(t, 0, l2.startCount)
	assert.Equal(t, 0, l2.stopCount)

	l1.wg.Add(1)
	l.Start()
	l1.wg.Wait()

	assert.Equal(t, 2, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}
