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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

type etwTester struct {
	etwStarted    chan struct{}
	loopStarted   chan struct{}
	stopLoop      chan struct{}
	loopExited    chan struct{}
	notify        chan interface{}
	p             *WindowsProbe
	notifications []interface{}
}

func createEtwTester(p *WindowsProbe) *etwTester {
	return &etwTester{
		etwStarted:    make(chan struct{}),
		loopStarted:   make(chan struct{}),
		stopLoop:      make(chan struct{}),
		loopExited:    make(chan struct{}),
		notify:        make(chan interface{}, 20),
		p:             p,
		notifications: make([]interface{}, 0),
	}
}
func (et *etwTester) runTestEtwFile() error {

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
		case etw.DDGUID(et.p.fileguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idCreate:
				if ca, err := parseCreateHandleArgs(e); err == nil {
					//fmt.Printf("Received idCreate event %d %v\n", e.EventHeader.EventDescriptor.ID, ca.string())
					select {
					case et.notify <- ca:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}
				}

			case idCreateNewFile:
				//fmt.Printf("Received event %d for PID %d\n", e.EventHeader.EventDescriptor.ID, e.EventHeader.ProcessID)
				if ca, err := parseCreateNewFileArgs(e); err == nil {
					//fmt.Printf("Received NewFile event %d %v\n", e.EventHeader.EventDescriptor.ID, ca.string())
					select {
					case et.notify <- ca:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}
				}
			case idCleanup:
				if ca, err := parseCleanupArgs(e); err == nil {
					//fmt.Printf("Received Cleanup event %d %v\n", e.EventHeader.EventDescriptor.ID, ca.string())
					select {
					case et.notify <- ca:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}
				}

			case idClose:
				if ca, err := parseCloseArgs(e); err == nil {
					//fmt.Printf("Received Close event %d %v\n", e.EventHeader.EventDescriptor.ID, ca.string())
					select {
					case et.notify <- ca:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}
					if e.EventHeader.EventDescriptor.ID == idClose {
						delete(filePathResolver, ca.fileObject)
					}
				}
			case idFlush:
				if fa, err := parseFlushArgs(e); err == nil {
					//fmt.Printf("got id %v args %s", e.EventHeader.EventDescriptor.ID, fa.string())
					select {
					case et.notify <- fa:
						// message sent
					default:
						// message dropped.  Which is OK.  In the test code, we want to leave the receive loop
						// running, but only catch messages when we're expecting them
						fmt.Printf("Dropped message\n")
					}

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

func processUntilClose(t *testing.T, et *etwTester) {

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
			case *closeArgs:
				return
			}
		}
	}

}

func stopLoop(et *etwTester, wg *sync.WaitGroup) {
	select {
	case et.stopLoop <- struct{}{}:

	default:
		// do nothing
	}

	wg.Wait()

	// do this little dance.  Once the wg is stopped, clear out the channel.
	// because the stopLoop above may have occurred after the loop exited,
	// and we want the channel clean for the next execution
	select {
	case <-et.stopLoop:
		break
	default:
		// we just want to poll and remove one if there is one
	}

}

// will leave file left over for use in future tests.
func testSimpleCreate(t *testing.T, et *etwTester, testfilename string) {

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		processUntilClose(t, et)
	}()
	// wait till we're sure the listening loop is running
	<-et.loopStarted

	f, err := os.Create(testfilename)
	assert.NoError(t, err)
	if err == nil {
		// don't remove as it will be used in subsequent test
		//defer os.Remove(testfilename)
	}
	f.Close()
	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		}
		return false
	}, 4*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	// now walk the list of notifications.

	// expect to see in this order
	// 12 (idCreate)
	// 30 (createNewFile)
	// 13 (cleanup)
	// 14 (close)

	assert.Equal(t, 4, len(et.notifications), "expected 4 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*createHandleArgs); ok {
		assert.True(t, isSameFile(testfilename, c.fileName), "expected %s, got %s", testfilename, c.fileName)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}

	if cf, ok := et.notifications[1].(*createNewFileArgs); ok {
		assert.True(t, isSameFile(testfilename, cf.fileName), "expected %s, got %s", testfilename, cf.fileName)
	} else {
		t.Errorf("expected createNewFileArgs, got %T", et.notifications[1])
	}

	if cu, ok := et.notifications[2].(*cleanupArgs); ok {
		assert.True(t, isSameFile(testfilename, cu.fileName), "expected %s, got %s", testfilename, cu.fileName)
	} else {
		t.Errorf("expected cleanupArgs, got %T", et.notifications[2])
	}

	if cl, ok := et.notifications[3].(*closeArgs); ok {
		assert.True(t, isSameFile(testfilename, cl.fileName), "expected %s, got %s", testfilename, cl.fileName)
	} else {
		t.Errorf("expected closeArgs, got %T", et.notifications[3])
	}
	et.notifications = et.notifications[:0]
}

func testFileOpen(t *testing.T, et *etwTester, testfilename string) {

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		processUntilClose(t, et)
	}()
	// wait till we're sure the listening loop is running
	<-et.loopStarted

	h, err := windows.CreateFile(windows.StringToUTF16Ptr(testfilename), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	require.NoError(t, err, "could not open file")
	assert.NotEqual(t, h, windows.InvalidHandle, "could not open file")
	windows.CloseHandle(h)

	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		}
		return false
	}, 4*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	// we expect a handle create (12), a cleanup and a close.
	assert.Equal(t, 3, len(et.notifications), "expected 4 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*createHandleArgs); ok {
		assert.True(t, isSameFile(testfilename, c.fileName), "expected %s, got %s", testfilename, c.fileName)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}

	if cu, ok := et.notifications[1].(*cleanupArgs); ok {
		assert.True(t, isSameFile(testfilename, cu.fileName), "expected %s, got %s", testfilename, cu.fileName)
	} else {
		t.Errorf("expected cleanupArgs, got %T", et.notifications[2])
	}

	if cl, ok := et.notifications[2].(*closeArgs); ok {
		assert.True(t, isSameFile(testfilename, cl.fileName), "expected %s, got %s", testfilename, cl.fileName)
	} else {
		t.Errorf("expected closeArgs, got %T", et.notifications[3])
	}
	et.notifications = et.notifications[:0]

}
func TestETWFileNotifications(t *testing.T) {
	ex, err := os.Executable()
	require.NoError(t, err, "could not get executable path")
	testfilename := ex + ".testfile"

	wp, err := createTestProbe()
	require.NoError(t, err)
	require.NotNil(t, wp)

	// teardownTestProe calls the stop function on etw, which will
	// in turn wait on wp.fimgw
	defer teardownTestProbe(wp)

	et := createEtwTester(wp)

	wp.fimwg.Add(1)
	go func() {
		defer wp.fimwg.Done()
		err := et.runTestEtwFile()
		assert.NoError(t, err)
	}()

	// wait until we're sure that the ETW listener is up and running.
	// as noted above, this _could_ cause an infinite deadlock if no notifications are received.
	// but, since we're getting the notifications from the entire system, we should be getting
	// a steady stream as soon as it's fired up.
	<-et.etwStarted

	t.Run("testSimpleCreate", func(t *testing.T) {
		testSimpleCreate(t, et, testfilename)
	})

	require.Equal(t, 0, len(et.notifications), "expected 0 notifications, got %d", len(et.notifications))
	// note the testFileOpen expects that the file is already created,
	// and left over from testSimpleCreate
	t.Run("testFileOpen", func(t *testing.T) {
		testFileOpen(t, et, testfilename)
	})
	assert.NoError(t, os.Remove(testfilename), "failed to remove")
}
