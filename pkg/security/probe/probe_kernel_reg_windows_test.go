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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// createTestProbe and teardownTestProbe are implemented in the file test, but
// the same one can be used.

func (p *WindowsProbe) runTestEtwRegistry(ch chan interface{}) error {
	mypid := os.Getpid()
	err := p.fimSession.StartTracing(func(e *etw.DDEventRecord) {

		// since this is for testing, skip any notification not from our pid
		if e.EventHeader.ProcessID != uint32(mypid) {
			return
		}
		switch e.EventHeader.ProviderID {
		case etw.DDGUID(p.regguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idRegCreateKey:
				if cka, err := parseCreateRegistryKey(e); err == nil {
					select {
					case ch <- cka:
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

func TestETWRegistryNotifications(t *testing.T) {
	os.Getpid()
	wp, err := createTestProbe()
	assert.NoError(t, err)
	assert.NotNil(t, wp)
	defer teardownTestProbe(wp)

	loopstarted := make(chan struct{})
	endloop := make(chan struct{})
	notifications := make(chan interface{})
	wp.fimwg.Add(1)
	go func() {
		defer wp.fimwg.Done()
		err := wp.runTestEtwRegistry(notifications)
		assert.NoError(t, err)
	}()

	//t.Run("testCreateFile", func(t *testing.T) {

	var notified atomic.Bool

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// listen for the notifications
		loopstarted <- struct{}{}
		for {
			select {
			case <-endloop:
				return

			case n := <-notifications:
				switch n.(type) {
				case *createKeyArgs:
					cka := n.(*createKeyArgs)
					assert.NotNil(t, cka)

					notified.Store(true)
					return
				}
			}
		}
	}()
	// wait till we're sure the listening loop is running
	<-loopstarted

	// create the key
	key, _, err := registry.CreateKey(windows.HKEY_CURRENT_USER, "Software\\Test", windows.KEY_READ|windows.KEY_WRITE)
	assert.NoError(t, err)
	if err == nil {
		defer key.Close()
	}
	assert.Eventually(t, func() bool {
		return notified.Load()
	}, 2*time.Second, 250*time.Millisecond, "did not get notification")

	select {
	case endloop <- struct{}{}:
		// message sent
	default:
		// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
		// running, but only catch messages when we're expecting them
	}
	wg.Wait()
	//})
}
