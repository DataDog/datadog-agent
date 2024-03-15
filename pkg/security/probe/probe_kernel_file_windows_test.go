// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

func createTestProbe() (*WindowsProbe, error) {

	opts := Opts{
		disableProcmon: true,
	}
	cfg, err := config.NewConfig()
	if err != nil {
		return nil, err
	}
	cfg.RuntimeSecurity.FIMEnabled = true

	// probe and config are provided as null.  During the tests, it is assumed
	// that we will not access those values.
	wp := &WindowsProbe{
		opts:   opts,
		config: cfg,
	}
	err = wp.Init()

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
			switch n.(type) {
			case *createHandleArgs, *createNewFileArgs, *cleanupArgs:
				et.notifications = append(et.notifications, n)
			case *closeArgs:
				et.notifications = append(et.notifications, n)
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

	h, err := windows.CreateFile(windows.StringToUTF16Ptr(testfilename),
		windows.GENERIC_READ,    // desired access
		windows.FILE_SHARE_READ, // share mode
		nil,                     // security attributes
		windows.OPEN_EXISTING,   //creation disposition
		0,                       // flags and attributes
		0)                       // template file

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

	// these options to not match the arguments to CreateFile.  these are the
	// kernel arguments, as documented in ZwCreateFile
	expectedCreateOptions := (kernelDisposition_FILE_OPEN << 24) | (kernelCreateOpts_FILE_NON_DIRECTORY_FILE | kernelCreateOpts_FILE_SYNCHRONOUS_IO_NONALERT)
	// we expect a handle create (12), a cleanup and a close.
	assert.Equal(t, 3, len(et.notifications), "expected 3 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*createHandleArgs); ok {
		assert.True(t, isSameFile(testfilename, c.fileName), "expected %s, got %s", testfilename, c.fileName)
		// this should be same as sharing argument to Createfile
		assert.Equal(t, uint32(windows.FILE_SHARE_READ), c.shareAccess, "Sharing mode did not match")
		assert.Equal(t, expectedCreateOptions, c.createOptions, "Create options did not match")

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
