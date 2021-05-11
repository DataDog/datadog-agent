// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import (
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/stretchr/testify/assert"
)

type mockLauncher struct {
	wg         sync.WaitGroup
	m          sync.Mutex
	isAvalible bool
	started    bool
	stopped    bool
}

func (m *mockLauncher) IsAvalible() bool {
	defer m.m.Unlock()
	m.m.Lock()
	return m.isAvalible
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
	l1 := mockLauncher{isAvalible: true}
	l2 := mockLauncher{isAvalible: false}

	l1.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()}, 1)
	l.Start()

	l1.wg.Wait()
	assert.True(t, l1.started)
	assert.False(t, l2.started)
	assert.Equal(t, l.activeLauncher, &l1)
}

func TestSelectSecond(t *testing.T) {
	l1 := mockLauncher{isAvalible: false}
	l2 := mockLauncher{isAvalible: true}

	l2.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()}, 1*time.Millisecond)
	l.Start()

	l2.wg.Wait()
	assert.False(t, l1.started)
	assert.True(t, l2.started)
	assert.Equal(t, l.activeLauncher, &l2)
}

func TestFailsThenSucceeds(t *testing.T) {
	l1 := mockLauncher{isAvalible: false}
	l2 := mockLauncher{isAvalible: false}

	l2.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()}, 1*time.Millisecond)
	l.Start()

	// let it run a few times
	time.Sleep(10 * time.Millisecond)

	assert.False(t, l1.started)
	assert.False(t, l2.started)
	assert.Equal(t, l.activeLauncher, nil)

	l2.SetAvalible(true)
	l2.wg.Wait()

	assert.False(t, l1.started)
	assert.True(t, l2.started)
	assert.Equal(t, l.activeLauncher, &l2)
}

func TestAvalibleLauncherReturnsNil(t *testing.T) {
	l1 := mockLauncher{isAvalible: false}
	l2 := mockLauncher{isAvalible: true}

	l2.wg.Add(1)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToErrLaunchable()}, 1*time.Millisecond)
	l.Start()

	l2.wg.Wait()
	assert.False(t, l1.started)
	assert.False(t, l2.started)
	_, ok := l.activeLauncher.(*noopLauncher)
	assert.True(t, ok)
}

func TestRestartUsesPreviousLauncher(t *testing.T) {
	l1 := mockLauncher{isAvalible: true}
	l2 := mockLauncher{isAvalible: false}

	l1.wg.Add(2)
	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()}, 1)
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
	l1 := mockLauncher{isAvalible: false}
	l2 := mockLauncher{isAvalible: false}

	l := NewLauncher([]ContainerLaunchable{l1.ToLaunchable(), l2.ToLaunchable()}, 1)
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
