// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"fmt"
	"os"
	"os/user"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// createTestProbe and teardownTestProbe are implemented in the file test, but
// the same one can be used.

func (et *etwTester) runTestEtwRegistry() error {

	var once sync.Once
	mypid := os.Getpid()
	err := et.p.fimSession.StartTracing(func(e *etw.DDEventRecord) {
		/*
			 	* this works because we're registered on the whole system.  Therefore, we'll get
			 	* some file or registry callback events from other processes we're not interested in.
				*
				* so sooner or later we'll get one.  If we don't, we'll deadlock in the test init routine below
		*/
		once.Do(func() {
			close(et.etwStarted)
		})

		// since this is for testing, skip any notification not from our pid
		if e.EventHeader.ProcessID != uint32(mypid) {
			return
		}
		switch e.EventHeader.ProviderID {
		case etw.DDGUID(et.p.regguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idRegCreateKey:
				if cka, err := parseCreateRegistryKey(e); err == nil {
					select {
					case et.notify <- cka:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}
				}
			case idRegOpenKey:
				if cka, err := parseCreateRegistryKey(e); err == nil {
					log.Debugf("Got idRegOpenKey %s", cka.string())
				}

			case idRegDeleteKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Infof("Got idRegDeleteKey %v", dka.string())
				}
			case idRegFlushKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Infof("Got idRegFlushKey %v", dka.string())
				}
			case idRegCloseKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Debugf("Got idRegCloseKey %s", dka.string())
					delete(regPathResolver, dka.keyObject)
				}
			case idQuerySecurityKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Infof("Got idQuerySecurityKey %v", dka.keyName)
				}
			case idSetSecurityKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Infof("Got idSetSecurityKey %v", dka.keyName)
				}
			case idRegSetValueKey:
				if svk, err := parseSetValueKey(e); err == nil {
					log.Infof("Got idRegSetValueKey %s", svk.string())
				}

			}
		}
	})
	return err
}

func processUntilRegOpen(t *testing.T, et *etwTester) {

	defer func() {
		et.loopExited <- struct{}{}
	}()
	et.loopStarted <- struct{}{}
	for {
		select {
		case <-et.stopLoop:
			return

		case n := <-et.notify:
			et.notifications = append(et.notifications, n)
			switch n.(type) {
			case *createKeyArgs:
				return
			}
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
		err := et.runTestEtwRegistry()
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
	expected := "\\REGISTRY\\USER\\" + sidstr + "\\" + keyname
	// create the key
	key, _, err := registry.CreateKey(windows.HKEY_CURRENT_USER, keyname, windows.KEY_READ|windows.KEY_WRITE)
	assert.NoError(t, err)
	if err == nil {
		defer key.Close()
	}

	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		}
		return false
	}, 4*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	assert.Equal(t, 1, len(et.notifications), "expected 1 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*createKeyArgs); ok {
		assert.Equal(t, expected, c.computedFullPath, "expected %s, got %s", expected, c.computedFullPath)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}
}
