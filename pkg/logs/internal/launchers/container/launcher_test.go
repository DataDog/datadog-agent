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

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/stretchr/testify/assert"
)

type mockLauncher struct {
	wg          sync.WaitGroup
	m           sync.Mutex
	retrier     retry.Retrier
	isAvailable bool
	startCount  int
	stopCount   int
	attempt     uint
}

func newMockLauncher(isAvailable bool) *mockLauncher {
	l := &mockLauncher{isAvailable: isAvailable}
	l.retrier.SetupRetrier(&retry.Config{
		Name:          "testing",
		AttemptMethod: func() error { return nil },
		Strategy:      retry.JustTesting,
		RetryCount:    0,
		RetryDelay:    time.Millisecond,
	})
	return l
}

func newMockLauncherBecomesAvailable(AvailableAfterRetries uint) *mockLauncher {
	l := &mockLauncher{isAvailable: false}
	l.retrier.SetupRetrier(&retry.Config{
		Name: "testing",
		AttemptMethod: func() error {
			l.attempt++
			if l.attempt > AvailableAfterRetries {
				l.isAvailable = true
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

func (m *mockLauncher) IsAvailable() (bool, *retry.Retrier) {
	defer m.m.Unlock()
	m.m.Lock()
	m.retrier.TriggerRetry()
	return m.isAvailable, &m.retrier
}

func (m *mockLauncher) SetAvailable(Available bool) {
	defer m.m.Unlock()
	m.m.Lock()
	m.isAvailable = Available
}

func (m *mockLauncher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	m.startCount++
	m.wg.Done()
}
func (m *mockLauncher) Stop() {
	m.stopCount++
}

func (m *mockLauncher) ToLaunchable() Launchable {
	return Launchable{
		IsAvailable: m.IsAvailable,
		Launcher: func() launchers.Launcher {
			m.wg.Done()
			return m
		},
	}
}

func (m *mockLauncher) ToErrLaunchable() Launchable {
	return Launchable{
		IsAvailable: m.IsAvailable,
		Launcher: func() launchers.Launcher {
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
	l.Start(nil, nil, nil)

	l1.wg.Wait()
	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestSelectSecond(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(true)

	l2.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start(nil, nil, nil)

	l2.wg.Wait()
	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 1, l2.startCount)
}

func TestFailsThenSucceeds(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(false)

	l2.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start(nil, nil, nil)

	// let it run a few times
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 0, l2.startCount)

	l2.SetAvailable(true)
	l2.wg.Wait()

	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 1, l2.startCount)
}

func TestFailsThenSucceedsRetrier(t *testing.T) {
	l1 := newMockLauncherBecomesAvailable(3)
	l2 := newMockLauncher(false)

	l1.wg.Add(3)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start(nil, nil, nil)

	l1.wg.Wait()

	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestAvailableLauncherReturnsNil(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(true)

	l2.wg.Add(1)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToErrLaunchable()})
	l.Start(nil, nil, nil)

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
	l.Start(nil, nil, nil)

	l1.wg.Wait()
	l.Stop()
	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 1, l1.stopCount)
	assert.Equal(t, 0, l2.startCount)
	assert.Equal(t, 0, l2.stopCount)

	l1.wg.Add(1)
	l.Start(nil, nil, nil)
	l1.wg.Wait()

	assert.Equal(t, 2, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestRestartFindLauncherLater(t *testing.T) {
	l1 := newMockLauncher(false)
	l2 := newMockLauncher(false)

	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start(nil, nil, nil)

	// let it run a few times
	time.Sleep(10 * time.Millisecond)
	l.Stop()

	assert.Equal(t, 0, l1.startCount)
	assert.Equal(t, 0, l1.stopCount)
	assert.Equal(t, 0, l2.startCount)
	assert.Equal(t, 0, l2.stopCount)

	l1.SetAvailable(true)

	l1.wg.Add(2)
	l.Start(nil, nil, nil)
	l1.wg.Wait()

	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}

func TestRestartSameLauncher(t *testing.T) {
	l1 := newMockLauncher(true)
	l2 := newMockLauncher(false)

	l1.wg.Add(2)
	l := NewLauncher([]Launchable{l1.ToLaunchable(), l2.ToLaunchable()})
	l.Start(nil, nil, nil)

	// let it run a few times
	time.Sleep(10 * time.Millisecond)
	l.Stop()
	l1.wg.Wait()

	assert.Equal(t, 1, l1.startCount)
	assert.Equal(t, 1, l1.stopCount)
	assert.Equal(t, 0, l2.startCount)
	assert.Equal(t, 0, l2.stopCount)

	l1.wg.Add(1)
	l.Start(nil, nil, nil)
	l1.wg.Wait()

	assert.Equal(t, 2, l1.startCount)
	assert.Equal(t, 0, l2.startCount)
}
