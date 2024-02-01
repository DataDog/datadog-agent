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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"

	"golang.org/x/sys/windows"
)

func createTestProbe() (*WindowsProbe, error) {

	opts := Opts{
		disableProcmon: true,
	}
	// probe and config are provided as null.  During the tests, it is assumed
	// that we will not access those values.
	wp := &WindowsProbe{
		opts: opts,
	}
	err := wp.Init()

	// do not call Start(), as start assumes we can load the driver.  these tests
	// are intended to be run without the driver needing to be present
	return wp, err
}

func teardownTestProbe(wp *WindowsProbe) {
	wp.Stop()
	// do not call Close(), as that expects the driver to be loaded.
}

func (p *WindowsProbe) runTestEtwFile(ch chan interface{}) error {
	mypid := os.Getpid()
	err := p.fimSession.StartTracing(func(e *etw.DDEventRecord) {

		// since this is for testing, skip any notification not from our pid
		if e.EventHeader.ProcessID != uint32(mypid) {
			return
		}
		switch e.EventHeader.ProviderID {
		case etw.DDGUID(p.fileguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idCreate:
				fallthrough
			case idCreateNewFile:
				//fmt.Printf("Received event %d for PID %d\n", e.EventHeader.EventDescriptor.ID, e.EventHeader.ProcessID)
				if ca, err := parseCreateArgs(e); err == nil {
					fmt.Printf("Received event %v\n", ca.string())
					select {
					case ch <- ca:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}
				}
			case idCleanup:
				fallthrough
			case idClose:
				fallthrough
			case idFlush:
				// don't fall through
				if ca, err := parseCleanupArgs(e); err == nil {
					log.Infof("got id %v args %s", e.EventHeader.EventDescriptor.ID, ca.string())
					delete(filePathResolver, ca.fileObject)
				}
			case idSetInformation:
				fallthrough
			case idSetDelete:
				fallthrough
			case idRename:
				fallthrough
			case idQueryInformation:
				fallthrough
			case idFSCTL:
				fallthrough
			case idRename29:
				if sia, err := parseInformationArgs(e); err == nil {
					log.Infof("got id %v args %s", e.EventHeader.EventDescriptor.ID, sia.string())
				}
			}
		}
	})
	return err
}

func isSameFile(drive, device string) bool {
	// if the file is not the one created, then skip it
	if strings.EqualFold(drive, device) {
		return true
	}
	// check to see if we got the \\device name
	driveletter := windows.StringToUTF16Ptr(drive[:2])

	tgtbuflen := windows.MAX_PATH // enough space for an \\device\\harddisk0\....

	buf := make([]uint16, tgtbuflen)
	windows.QueryDosDevice(driveletter, &buf[0], uint32(tgtbuflen))
	devicestring := windows.UTF16ToString(buf)

	cmpstr := strings.Replace(device, devicestring, drive[:2], 1)

	return strings.EqualFold(cmpstr, drive)

}
func TestETWFileNotifications(t *testing.T) {
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
		err := wp.runTestEtwFile(notifications)
		assert.NoError(t, err)
	}()

	var file *os.File

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
				case *createArgs:
					ca := n.(*createArgs)
					assert.NotNil(t, ca)

					if !isSameFile(file.Name(), ca.fileName) {
						continue
					}
					notified.Store(true)
					return
				}
			}
		}
	}()
	// wait till we're sure the listening loop is running
	<-loopstarted
	//time.Sleep(2 * time.Second)

	file, err = os.CreateTemp("", "test")
	assert.NoError(t, err)
	if err == nil {
		defer os.Remove(file.Name())
	}
	if !assert.Eventually(t, func() bool {
		return notified.Load()
	}, 4*time.Second, 250*time.Millisecond, "did not get notification") {
		outbuf := make([]byte, 819200)
		runtime.Stack(outbuf, true)
		t.Logf("Stack trace:\n%s", string(outbuf))
	}

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
