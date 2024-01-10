// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

//nolint:revive // TODO(WKIT) Fix revive linter
package procmon

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/driver"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/olreader"
)

//nolint:revive // TODO(WKIT) Fix revive linter
type ProcessStartNotification struct {
	Pid               uint64
	PPid              uint64
	CreatingProcessId uint64
	CreatingThreadId  uint64
	OwnerSidString    string
	ImageFile         string
	CmdLine           string
	// if this is nonzero, functions as notification to
	// the probe that the buffer size isn't large enough
	RequiredSize uint32
}

//nolint:revive // TODO(WKIT) Fix revive linter
type ProcessStopNotification struct {
	Pid uint64
}

//nolint:revive // TODO(WKIT) Fix revive linter
type WinProcmon struct {
	onStart chan *ProcessStartNotification
	onStop  chan *ProcessStopNotification
	onError chan bool

	reader *olreader.OverlappedReader
}

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddprocmon`
	// driverName is the name of the driver service
	driverName = "ddprocmon"

	// default size of the receive buffer
	procmonReceiveSize = 4096

	// number of buffers
	procmonNumBufs = 50
)

//nolint:revive // TODO(WKIT) Fix revive linter
func NewWinProcMon(onStart chan *ProcessStartNotification, onStop chan *ProcessStopNotification, onError chan bool) (*WinProcmon, error) {

	wp := &WinProcmon{
		onStart: onStart,
		onStop:  onStop,
		onError: onError,
	}
	if err := driver.StartDriverService(driverName); err != nil {
		return nil, err
	}
	reader, err := olreader.NewOverlappedReader(wp, procmonReceiveSize, procmonNumBufs)
	if err != nil {
		return nil, err
	}
	err = reader.Open(deviceName)
	if err != nil {
		return nil, err
	}
	wp.reader = reader
	return wp, nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (wp *WinProcmon) OnData(data []uint8) {
	var consumed uint32
	returnedsize := uint32(len(data))
	start, stop := decodeStruct(data[consumed:], returnedsize-consumed)

	if start != nil {

		wp.onStart <- start
	} else if stop != nil {

		wp.onStop <- stop
	}
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (wp *WinProcmon) OnError(err error) {

	// if we get this error notification, then the driver can't continue.
	// stop the notifications so that the driver can't get backed up
	wp.Stop()
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (wp *WinProcmon) Stop() {
	// since we're stopping, if for some reason this ioctl fails, there's nothing we can
	// do, we're on our way out.  Closing the handle will ultimately cause the same cleanup
	// to happen.
	_ = wp.reader.Ioctl(ProcmonStopIOCTL,
		nil, // inBuffer
		0,
		nil,
		0,
		nil,
		nil)
	wp.reader.Stop()

	_ = driver.StopDriverService(driverName, false)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (wp *WinProcmon) Start() error {
	err := wp.reader.Read()
	if err != nil {
		return err
	}
	// this will initiate the driver actually sending things up
	// start grabbing notifications
	err = wp.reader.Ioctl(ProcmonStartIOCTL,
		nil, // inBuffer
		0,
		nil,
		0,
		nil,
		nil)
	if err != nil {
		wp.reader.Stop()
	}
	return err
}

//nolint:revive // TODO(WKIT) Fix revive linter
func decodeStruct(data []uint8, sz uint32) (start *ProcessStartNotification, stop *ProcessStopNotification) {
	start = nil
	stop = nil
	if unsafe.Sizeof(DDProcessNotification{}.Size) > uintptr(sz) {
		return nil, nil
	}

	n := *(*DDProcessNotification)(unsafe.Pointer(&data[0]))
	if n.Size > uint64(sz) {
		return nil, nil
	}

	t := DDProcessNotifyType(n.NotifyType)

	if t == ProcmonNotifyStart {
		var imagefile string
		var cmdline string
		var sidstring string

		if n.ImageFileLen > 0 {
			imagefile = winutil.ConvertWindowsString(data[n.ImageFileOffset : n.ImageFileOffset+n.ImageFileLen])
		}
		if n.CommandLineLen > 0 {
			cmdline = winutil.ConvertWindowsString(data[n.CommandLineOffset : n.CommandLineOffset+n.CommandLineLen])
		}

		if n.SidLen > 0 {
			sidstring = winutil.ConvertWindowsString(data[n.SidOffset : n.SidOffset+n.SidLen])
		}
		start = &ProcessStartNotification{
			Pid:               n.ProcessId,
			PPid:              n.ParentProcessId,
			CreatingProcessId: n.CreatingProcessId,
			CreatingThreadId:  n.CreatingThreadId,
			ImageFile:         imagefile,
			CmdLine:           cmdline,
			OwnerSidString:    sidstring,
		}
		if n.SizeNeeded > n.Size {
			start.RequiredSize = uint32(n.SizeNeeded)
		}

		return start, nil
	} else if t == ProcmonNotifyStop {
		stop = &ProcessStopNotification{
			Pid: n.ProcessId,
		}
		return nil, stop
	}
	return nil, nil
}
