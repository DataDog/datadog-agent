// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"

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

	wp, err := initializeWindowsProbe(cfg, opts)
	if err != nil {
		return nil, err
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

func processUntil(t *testing.T, et *etwTester, target interface{}, count int) {

	et.notifications = et.notifications[:0]
	defer func() {
		et.loopExited <- struct{}{}
	}()
	et.loopStarted <- struct{}{}

	var targetcount int
	for {
		select {
		case <-et.stopLoop:
			return

		case n := <-et.notify:
			switch n.(type) {
			case *createHandleArgs, *createNewFileArgs, *cleanupArgs, *closeArgs, *writeArgs, *setDeleteArgs, *deletePathArgs, *renameArgs, *renamePath:
				et.notifications = append(et.notifications, n)
				if reflect.TypeOf(n) == reflect.TypeOf(target) {
					targetcount++
					if targetcount >= count {
						return
					}
				}
			}
		}
	}
}

func processUntilAllClosed(t *testing.T, et *etwTester) {

	et.notifications = et.notifications[:0]
	defer func() {
		et.loopExited <- struct{}{}
	}()
	et.loopStarted <- struct{}{}

	var opencount int
	var closecount int
	for {
		select {
		case <-et.stopLoop:
			return

		case n := <-et.notify:
			notify := false
			switch n.(type) {
			case *createHandleArgs, *createNewFileArgs:
				opencount++
				notify = true

			case *closeArgs:
				closecount++
				notify = true

			case *cleanupArgs, *writeArgs, *setDeleteArgs, *deletePathArgs, *renameArgs, *renamePath:
				notify = true

			default:
				// do nothing
			}
			if notify {
				et.notifications = append(et.notifications, n)
				if opencount > 0 && opencount == closecount {
					return
				}
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
		processUntil(t, et, &closeArgs{}, 1)
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
		default:
			return false
		}
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

// will leave file left over for use in future tests.
func testSimpleFileWrite(t *testing.T, et *etwTester, testfilename string) {

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		/*
			I don't know why this is, but...
			empirically, the kernel holds on to the close notification for long after the CloseHandle()
			(I tried with the native closehandle API, same behavior), at least when there was a write on the
			file (above, in the simple create test, it closes immediately).

			the IRP_MJ_CLEANUP docs say that this is sent when the refcount on the handle has reached zero
			(all handles closed).  So for this test it works fine, waiting for the close notification fails the
			test
		*/
		processUntil(t, et, &cleanupArgs{}, 1)
	}()
	// wait till we're sure the listening loop is running
	<-et.loopStarted

	t.Logf("================\n")
	// this will truncate the already existing file, that's OK.
	f, err := os.OpenFile(testfilename, os.O_RDWR, 0666)
	assert.NoError(t, err)
	if err == nil {
		// don't remove as it will be used in subsequent test
		//defer os.Remove(testfilename)
	}
	f.WriteString("hello")
	f.Close()

	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		default:
			return false
		}
	}, 10*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	// now walk the list of notifications.

	// expect to see in this order
	// (idCreate)
	// (write)
	// (cleanup)

	assert.Equal(t, 3, len(et.notifications), "expected 3 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*createHandleArgs); ok {
		assert.True(t, isSameFile(testfilename, c.fileName), "expected %s, got %s", testfilename, c.fileName)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}

	if wa, ok := et.notifications[1].(*writeArgs); ok {
		assert.True(t, isSameFile(testfilename, wa.fileName), "expected %s, got %s", testfilename, wa.fileName)
		assert.Equal(t, uint32(5), wa.IOSize, "expected 5, got %d", wa.IOSize)
	} else {
		t.Errorf("expected writeArgs, got %T", et.notifications[1])
	}

	if cl, ok := et.notifications[2].(*cleanupArgs); ok {
		assert.True(t, isSameFile(testfilename, cl.fileName), "expected %s, got %s", testfilename, cl.fileName)
	} else {
		t.Errorf("expected cleanup, got %T", et.notifications[2])
	}
	et.notifications = et.notifications[:0]
}

func testSimpleFileDelete(t *testing.T, et *etwTester, testfilename string) {

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		processUntil(t, et, &closeArgs{}, 1)
	}()
	// wait till we're sure the listening loop is running
	<-et.loopStarted
	et.notifications = et.notifications[:0]
	t.Logf("================\n")
	t.Logf("Deleting %s\n", testfilename)
	err := os.Remove(testfilename)
	assert.NoError(t, err)
	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		default:
			return false
		}
	}, 10*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	// now walk the list of notifications.

	// expect to see in this order
	// (idCreate)
	// (set_delete)
	// (cleanup)
	/*
		assert.Equal(t, 2, len(et.notifications), "expected 2 notifications, got %d", len(et.notifications))
		if len(et.notifications) < 3 {
			for _, n := range et.notifications {
				t.Logf("notification: %s\n", n)
			}
			t.Errorf("expected 3 notifications, got %d", len(et.notifications))
			return
		}
	*/
	if c, ok := et.notifications[0].(*createHandleArgs); ok {
		assert.True(t, isSameFile(testfilename, c.fileName), "expected %s, got %s", testfilename, c.fileName)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}

	if wa, ok := et.notifications[1].(*setDeleteArgs); ok {
		assert.True(t, isSameFile(testfilename, wa.fileName), "expected %s, got %s", testfilename, wa.fileName)
	} else {
		t.Errorf("expected setDelete, got %T", et.notifications[1])
	}

	/*
		if cl, ok := et.notifications[2].(*cleanupArgs); ok {
			assert.True(t, isSameFile(testfilename, cl.fileName), "expected %s, got %s", testfilename, cl.fileName)
		} else {
			t.Errorf("expected cleanup, got %T", et.notifications[2])
		}
	*/
	et.notifications = et.notifications[:0]
}

func testSimpleFileRename(t *testing.T, et *etwTester, testfilename, testfilerename string) {

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		//processUntil(t, et, &renamePath{}, 1)
		processUntilAllClosed(t, et)
	}()
	// wait till we're sure the listening loop is running
	<-et.loopStarted

	t.Logf("================\n")
	err := os.Rename(testfilename, testfilerename)
	assert.NoError(t, err)
	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		default:
			return false
		}
	}, 10*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)

	// now walk the list of notifications.

	// expect to see in this order
	// (idCreate) (on orig file)
	// (rename) (on orig file)
	// (rename_path) on new file

	//assert.Equal(t, 4, len(et.notifications), "expected 4 notifications, got %d", len(et.notifications))

	if c, ok := et.notifications[0].(*createHandleArgs); ok {
		assert.True(t, isSameFile(testfilename, c.fileName), "expected %s, got %s", testfilename, c.fileName)
	} else {
		t.Errorf("expected createHandleArgs, got %T", et.notifications[0])
	}

	// there are a variable number of notifications depending on OS.  FOr some reason, at least on Win11
	// it opens the root of the FS in the middle.  Just scan forward until we hit the rename.

	var ndx int
	for ndx = 1; ndx < len(et.notifications); ndx++ {
		if ra, ok := et.notifications[ndx].(*renameArgs); ok {
			assert.True(t, isSameFile(testfilename, ra.fileName), "expected %s, got %s", testfilename, ra.fileName)
			break
		}
	}

	// now, check that the next one is the renamePath
	require.Less(t, ndx+1, len(et.notifications), "expected renamePath, got nothing")
	ndx++
	// now expecte two creates on the new file name.  IDK why two
	if c, ok := et.notifications[ndx].(*renamePath); ok {
		assert.True(t, isSameFile(testfilerename, c.filePath), "expected %s, got %s", testfilerename, c.filePath)
		assert.True(t, isSameFile(testfilename, c.oldPath), "expected %s, got %s", testfilename, c.oldPath)
	} else {
		t.Errorf("expected renamePath, got %T", et.notifications[ndx])
	}
	et.notifications = et.notifications[:0]
}

func testFileOpen(t *testing.T, et *etwTester, testfilename string) {

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		processUntil(t, et, &closeArgs{}, 1)
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
		default:
			return false
		}
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
	if false {
		ebpftest.LogLevel(t, "info")
	}
	ex, err := os.Executable()
	require.NoError(t, err, "could not get executable path")
	testfilename := ex + ".testfile"
	testfilerename := ex + ".testfilerename"

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

	t.Run("testSimpleCreate", func(t *testing.T) {
		testSimpleCreate(t, et, testfilename)
	})
	require.Equal(t, 0, len(et.notifications), "expected 0 notifications, got %d", len(et.notifications))
	// note the testFileOpen expects that the file is already created,
	// and left over from testSimpleCreate
	t.Run("testFileOpen", func(t *testing.T) {
		testFileOpen(t, et, testfilename)
	})

	// this test assumes that the file is still there from previous tests.  If it's not,
	// it will fail because the sequence of messages is different.
	t.Run("testSimpleFileWrite", func(t *testing.T) {
		testSimpleFileWrite(t, et, testfilename)
	})
	t.Run("testSimpleFileRename", func(t *testing.T) {
		testSimpleFileRename(t, et, testfilename, testfilerename)
	})

	// this test assumes the file is still there from previous ones.
	// it will delete the file; therefore any subsequent test requiring the file will fail
	t.Run("testSimpleFileDelete", func(t *testing.T) {
		testSimpleFileDelete(t, et, testfilerename)
	})
}
