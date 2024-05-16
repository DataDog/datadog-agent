// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

func processUntilLoadNotification(t *testing.T, et *etwTester) {

	et.notifications = et.notifications[:0]
	defer func() {
		et.loopExited <- struct{}{}
	}()
	et.loopStarted <- struct{}{}

	for {
		select {
		case <-et.stopLoop:
			return

		case n := <-et.notify:
			switch n.(type) {
			case *imageLoadArgs:
				et.notifications = append(et.notifications, n)
				return
			}
		}
	}
}

func TestModuleLoadNotifications(t *testing.T) {
	if true {
		ebpftest.LogLevel(t, "info")
	}
	wp, err := createTestProbe()
	require.NoError(t, err)

	// teardownTestProe calls the stop function on etw, which will
	// in turn wait on wp.fimgw
	defer teardownTestProbe(wp)

	et := createEtwTester(wp)

	wp.fimwg.Add(1)
	go func() {
		defer wp.fimwg.Done()

		var once sync.Once
		mypid := os.Getpid()

		err := et.p.setupEtw(func(n interface{}, pid uint32) {
			once.Do(func() {
				close(et.etwStarted)
			})
			if pid != uint32(mypid) {
				return
			}
			select {
			case et.notify <- n:
				// message sent
			default:
			}
		})
		assert.NoError(t, err)
	}()
	// wait until we're sure that the ETW listener is up and running.
	// as noted above, this _could_ cause an infinite deadlock if no notifications are received.
	// but, since we're getting the notifications from the entire system, we should be getting
	// a steady stream as soon as it's fired up.
	<-et.etwStarted
	t.Run("testModuleLoadOnSelf", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			processUntilLoadNotification(t, et)
		}()
		<-et.loopStarted
		h, err := windows.LoadLibrary("comctl32.dll")
		assert.NoError(t, err)
		defer windows.FreeLibrary(h)
		assert.Eventually(t, func() bool {
			select {
			case <-et.loopExited:
				return true
			default:
				return false
			}
		}, 4*time.Second, 250*time.Millisecond, "did not get notification")
		stopLoop(et, &wg)

		assert.Equal(t, 1, len(et.notifications), "expected 1 notification, got %d", len(et.notifications))
		if ila, ok := et.notifications[0].(*imageLoadArgs); ok {
			// because of sxs, not sure _exactly_ where it will be loaded from.
			// just ensure the .DLL part is right
			base := filepath.Base(ila.imageName)
			assert.True(t, isSameFile("comctl32.dll", base), "expected comctl32.dll, got %s", base)
		} else {
			t.Errorf("expected imageLoadArgs, got %T", et.notifications[0])
		}

	})

}
