// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"strings"
	"sync"


	"github.com/DataDog/datadog-agent/pkg/security/config"

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
	wp.isRenameEnabled = true
	wp.isDeleteEnabled = true
	wp.isWriteEnabled = true

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

