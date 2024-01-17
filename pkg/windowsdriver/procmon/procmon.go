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
}

//nolint:revive // TODO(WKIT) Fix revive linter
type ProcessStopNotification struct {
	Pid uint64
}

//nolint:revive // TODO(WKIT) Fix revive linter
type WinProcmon struct {
	onStart chan *ProcessStartNotification
	onStop  chan *ProcessStopNotification

	reader *olreader.OverlappedReader
}

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddprocmon`
	// driverName is the name of the driver service
	driverName = "ddprocmon"

	// read buffer size provided to driver.  Must be large enough to contain entire buffer
	readBufferSize = 4096

	// number of buffers to pre-allocate
	numReadBuffers = 100
)

//nolint:revive // TODO(WKIT) Fix revive linter
func NewWinProcMon(onStart chan *ProcessStartNotification, onStop chan *ProcessStopNotification) (*WinProcmon, error) {

	wp := &WinProcmon{
		onStart: onStart,
		onStop:  onStop,
	}
	if err := driver.StartDriverService(driverName); err != nil {
		return nil, err
	}
	reader, err := olreader.NewOverlappedReader(wp, readBufferSize, numReadBuffers)
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
	for consumed < returnedsize {
		start, stop, used := decodeStruct(data[consumed:], returnedsize-consumed)
		if used == 0 {
			break
		}

		consumed += used
		if start != nil {

			wp.onStart <- start
		} else if stop != nil {

			wp.onStop <- stop
		}
	}
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (wp *WinProcmon) OnError(err error) {

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
func decodeStruct(data []uint8, sz uint32) (start *ProcessStartNotification, stop *ProcessStopNotification, consumed uint32) {
	start = nil
	stop = nil
	if unsafe.Sizeof(DDProcessNotification{}.Size) > uintptr(sz) {
		return nil, nil, 0
	}

	n := *(*DDProcessNotification)(unsafe.Pointer(&data[0]))
	if n.Size > uint64(sz) {
		return nil, nil, 0
	}

	consumed = uint32(n.Size)
	t := DDProcessNotifyType(n.NotifyType)

	if t == ProcmonNotifyStart {
		imagefile := winutil.ConvertWindowsString(data[n.ImageFileOffset : n.ImageFileOffset+n.ImageFileLen])
		cmdline := winutil.ConvertWindowsString(data[n.CommandLineOffset : n.CommandLineOffset+n.CommandLineLen])
		var sidstring string
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
	} else if t == ProcmonNotifyStop {
		stop = &ProcessStopNotification{
			Pid: n.ProcessId,
		}
	}
	return
}
