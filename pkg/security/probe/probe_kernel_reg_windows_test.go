// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"os"
	"os/user"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func processUntilRegOpen(t *testing.T, et *etwTester) {

	skippedObjects := make(map[fileObjectPointer]struct{})
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
			case *openKeyArgs:
				if strings.HasPrefix(n.(*openKeyArgs).computedFullPath, "HKEY_USERS\\") {
					et.notifications = append(et.notifications, n)
				}
				continue

			case *createKeyArgs:
				if strings.HasPrefix(n.(*createKeyArgs).computedFullPath, "HKEY_USERS\\") {
					et.notifications = append(et.notifications, n)
					return
				}
				continue

			case *createHandleArgs:
				ca := n.(*createHandleArgs)
				// we get all sorts of notifications of DLLs being loaded.
				// skip those

				// check the last 4 chars of the filename
				if l := len(ca.fileName); l >= 4 {
					// see if it's a .dll
					ext := ca.fileName[l-4:]

					// check to see if it's a dll
					if strings.EqualFold(ext, ".dll") {
						skippedObjects[ca.fileObject] = struct{}{}
						// don't add
						continue
					}
				}
			case *cleanupArgs:
				ca := n.(*cleanupArgs)

				// check to see if we already saw the createHandle for this, and if
				// so, just skip
				if _, ok := skippedObjects[ca.fileObject]; ok {
					continue
				}
			case *closeArgs:
				ca := n.(*closeArgs)
				// check to see if we already saw the createHandle for this, and if
				// so, just skip
				if _, ok := skippedObjects[ca.fileObject]; ok {
					// remove it from the map, since it's being closed.  it could be
					// reused.
					delete(skippedObjects, ca.fileObject)
					continue
				}

			}
			et.notifications = append(et.notifications, n)
		}
	}

}
func TestETWRegistryNotifications(t *testing.T) {
	wp, err := createTestProbe()
	require.NoError(t, err)
	require.NotNil(t, wp)

	// get the current user sid, since we're going to use the current user registry.
	u, err := user.Current()
	require.NoError(t, err)

	sidstr := u.Uid

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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		processUntilRegOpen(t, et)
	}()

	// wait until we're sure that the ETW listener is up and running.
	// as noted above, this _could_ cause an infinite deadlock if no notifications are received.
	// but, since we're getting the notifications from the entire system, we should be getting
	// a steady stream as soon as it's fired up.
	<-et.etwStarted
	<-et.loopStarted

	keyname := "Software\\Test"
	expectedBase := "HKEY_USERS\\" + sidstr
	expected := expectedBase + "\\" + keyname
	key, _, err := registry.CreateKey(windows.HKEY_CURRENT_USER, keyname, windows.KEY_READ|windows.KEY_WRITE)
	assert.NoError(t, err)
	if err == nil {
		defer key.Close()
	}

	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		default:
			return false
		}
	}, 4*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	assert.Equal(t, 2, len(et.notifications), "expected 2 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*openKeyArgs); ok {
		assert.Equal(t, expectedBase, c.computedFullPath, "expected %s, got %s", expectedBase, c.computedFullPath)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}
	if c, ok := et.notifications[1].(*createKeyArgs); ok {
		assert.Equal(t, expected, c.computedFullPath, "expected %s, got %s", expected, c.computedFullPath)
	} else {
		t.Errorf("expected createKeyArgs, got %T", et.notifications[1])
	}
}
