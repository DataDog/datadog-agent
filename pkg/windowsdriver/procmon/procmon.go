// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procmon

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/olreader"
)

type ProcessStartNotification struct {
	Pid       uint64
	PPid      uint64
	ImageFile string
	CmdLine   string
}

type ProcessStopNotification struct {
	Pid uint64
}

type WinProcmon struct {
	onStart chan *ProcessStartNotification
	onStop  chan *ProcessStopNotification

	reader *olreader.OverlappedReader
}

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddprocmon`
)

func NewWinProcMon(onStart chan *ProcessStartNotification, onStop chan *ProcessStopNotification) (*WinProcmon, error) {

	wp := &WinProcmon{
		onStart: onStart,
		onStop:  onStop,
	}
	reader, err := olreader.NewOverlappedReader(wp, 1024, 100)
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

func (wp *WinProcmon) OnData(data []uint8) {
	var consumed uint32
	returnedsize := uint32(len(data))
	for consumed < returnedsize {
		t, pid, img, cmd, used := decodeStruct(data[consumed:], returnedsize-consumed)
		consumed += used
		if t == ProcmonNotifyStart {

			// for now, calculate PPID here in user mode.
			// by calculating here, we can replace with kernel mode later and no
			// downstream code will have to change
			// TODO.  Add parent pid
			ppid, err := procutil.GetParentPid(uint32(pid))
			if err != nil {
				ppid = 0
			}
			s := &ProcessStartNotification{
				Pid:       pid,
				PPid:      uint64(ppid),
				ImageFile: img,
				CmdLine:   cmd,
			}
			wp.onStart <- s
		} else if t == ProcmonNotifyStop {
			s := &ProcessStopNotification{
				Pid: pid,
			}
			wp.onStop <- s
		}
	}
}

func (wp *WinProcmon) OnError(err error) {

}
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
}
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

func decodeStruct(data []uint8, sz uint32) (t DDProcessNotifyType, pid uint64, imagefile, cmdline string, consumed uint32) {
	n := *(*DDProcessNotification)(unsafe.Pointer(&data[0]))

	consumed = uint32(n.Size)
	pid = uint64(n.ProcessId)
	t = DDProcessNotifyType(n.NotifyType)

	if t == ProcmonNotifyStart {
		imagefile = winutil.ConvertWindowsString(data[n.ImageFileOffset : n.ImageFileOffset+n.ImageFileLen])
		cmdline = winutil.ConvertWindowsString(data[n.CommandLineOffset : n.CommandLineOffset+n.CommandLineLen])
	}
	return
}
