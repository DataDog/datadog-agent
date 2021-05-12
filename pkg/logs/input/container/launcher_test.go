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
	started    bool
	stopped    bool
	attempt    uint
}

func NewMockLauncher(isAvalible bool) *mockLauncher {
	l := &mockLauncher{isAvalible: isAvalible}
	l.retrier.SetupRetrier(&retry.Config{
		Name:          "testing",
		AttemptMethod: func() error { return nil },
		Strategy:      retry.JustTesting,
		RetryCount:    5,
		RetryDelay:    time.Millisecond,
	})
	return l
}

func NewMockLauncherBecomesAvalible(avalibleAfterRetries uint) *mockLauncher {
	l := &mockLauncher{isAvalible: false}
	l.retrier.SetupRetrier(&retry.Config{
		Name: "testing",
		AttemptMethod: func() error {
			l.attempt += 1
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
	m.started = true
	m.wg.Done()
}
func (m *mockLauncher) Stop() {
	m.stopped = true
}

func (m *mockLauncher) ToLaunchable() ContainerLaunchable {
	return ContainerLaunchable{
		IsAvailble: m.IsAvalible,
		Launcher: func() restart.Restartable {
			m.wg.Done()
			return m
		},
	}
}

func (m *mockLauncher) ToErrLaunchable() ContainerLaunchable {
	return ContainerLaunchable{
		IsAvailble: m.IsAvalible,
		Launcher: func() restart.Restartable {
			m.wg.Done()
			return nil
		},
	}
}

func TestSelectFirst(t *testing.T) {
	l1 := NewMockLauncher(true)
	l2 := NewMockLauncher(false)

	l1.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l1.wg.Wait()
	assert.True(t, l1.started)
	assert.False(t, l2.started)
	assert.NotNil(t, l.activeLauncher)
}

func TestSelectSecond(t *testing.T) {
	l1 := NewMockLauncher(false)
	l2 := NewMockLauncher(true)

	l2.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l2.wg.Wait()
	assert.False(t, l1.started)
	assert.True(t, l2.started)
	assert.NotNil(t, l.activeLauncher)
}

func TestFailsThenSucceeds(t *testing.T) {
	l1 := NewMockLauncher(false)
	l2 := NewMockLauncher(false)

	l2.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	// let it run a few times
	time.Sleep(10 * time.Millisecond)

	assert.False(t, l1.started)
	assert.False(t, l2.started)
	assert.Nil(t, l.activeLauncher)

	l2.SetAvalible(true)
	l2.wg.Wait()

	assert.False(t, l1.started)
	assert.True(t, l2.started)
	assert.NotNil(t, l.activeLauncher)
}

func TestFailsThenSucceedsRetrier(t *testing.T) {
	l1 := NewMockLauncherBecomesAvalible(3)
	l2 := NewMockLauncher(false)

	l1.wg.Add(3)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l1.wg.Wait()

	assert.True(t, l1.started)
	assert.False(t, l2.started)
	assert.NotNil(t, l.activeLauncher)
}

func TestAvalibleLauncherReturnsNil(t *testing.T) {
	l1 := NewMockLauncher(false)
	l2 := NewMockLauncher(true)

	l2.wg.Add(1)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToErrLaunchable()})
	l.Start()

	l2.wg.Wait()
	assert.False(t, l1.started)
	assert.False(t, l2.started)
	_, ok := l.activeLauncher.(*noopLauncher)
	assert.True(t, ok)
}

func TestRestartUsesPreviousLauncher(t *testing.T) {
	l1 := NewMockLauncher(true)
	l2 := NewMockLauncher(false)

	l1.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	l1.wg.Wait()
	l.Stop()
	assert.True(t, l1.started)
	assert.True(t, l1.stopped)
	assert.False(t, l2.stopped)

	l1.started = false

	l1.wg.Add(1)
	l.Start()
	l1.wg.Wait()

	assert.True(t, l1.started)
	assert.False(t, l2.started)
}

func TestRestartFindLauncherLater(t *testing.T) {
	l1 := NewMockLauncher(false)
	l2 := NewMockLauncher(false)

	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start()

	// let it run a few times
	time.Sleep(10 * time.Millisecond)
	l.Stop()
	time.Sleep(10 * time.Millisecond)

	assert.False(t, l1.started)
	assert.False(t, l1.stopped)
	assert.False(t, l2.started)
	assert.False(t, l2.stopped)

	l1.isAvalible = true

	l1.wg.Add(2)
	l.Start()
	l1.wg.Wait()

	assert.True(t, l1.started)
	assert.False(t, l2.started)
}
