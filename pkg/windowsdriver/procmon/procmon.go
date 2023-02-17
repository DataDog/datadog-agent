// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package procmon

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
)

type ProcessStartNotification struct {
	Pid       uint64
	ImageFile string
	CmdLine   string
}

type ProcessStopNotification struct {
	Pid uint64
}

type WinProcmon struct {
	h       *Handle
	onStart chan *ProcessStartNotification
	onStop  chan *ProcessStopNotification

	wg         sync.WaitGroup
	ctx        context.Context
	cancelFunc context.CancelFunc
}
type Handle struct {
	windows.Handle
}

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddprocmon`
)

func NewWinProcMon(onStart chan *ProcessStartNotification, onStop chan *ProcessStopNotification) (*WinProcmon, error) {
	h, err := newHandle(0)
	if err != nil {
		return nil, fmt.Errorf("Failed to create driver handle %v", err)
	}

	ctx, cancelFnc := context.WithCancel(context.Background())
	wp := &WinProcmon{
		h:          h,
		onStart:    onStart,
		onStop:     onStop,
		ctx:        ctx,
		cancelFunc: cancelFnc,
	}
	return wp, nil
}

func (wp *WinProcmon) Stop() {
	wp.cancelFunc()
	wp.wg.Wait()
	windows.CloseHandle(wp.h.Handle)
}
func (wp *WinProcmon) Start() error {
	wp.wg.Add(1)
	go func() {
		defer wp.wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-wp.ctx.Done():
				return
			case <-ticker.C:
				readbuffer := make([]uint8, 1024)
				found := true
				for found {
					found = false
					var returnedsize uint32
					err := windows.DeviceIoControl(wp.h.Handle,
						GetNewProcsIOCTL,
						nil,
						0,
						(*byte)(unsafe.Pointer(&readbuffer[0])),
						1024,
						&returnedsize,
						nil)

					if err == nil && returnedsize > 0 {
						var consumed uint32
						for consumed < returnedsize {
							pid, img, cmd, used := decodeStruct(readbuffer[consumed:], returnedsize-consumed)
							consumed += used
							s := &ProcessStartNotification{
								Pid:       pid,
								ImageFile: img,
								CmdLine:   cmd,
							}
							wp.onStart <- s
						}
					}
					err = windows.DeviceIoControl(wp.h.Handle,
						GetDeadProcsIOCTL,
						nil,
						0,
						(*byte)(unsafe.Pointer(&readbuffer[0])),
						1024,
						&returnedsize,
						nil)
					if err == nil && returnedsize > 0 {
						var consumed uint32
						for consumed < returnedsize {
							pid, _, _, used := decodeStruct(readbuffer[consumed:], returnedsize-consumed)
							consumed += used
							s := &ProcessStopNotification{
								Pid: pid,
							}
							wp.onStop <- s
						}
					}
				}
			}
		}
	}()
	return nil
}

// NewHandle creates a new windows handle attached to the driver
func newHandle(flags uint32) (*Handle, error) {
	p, err := windows.UTF16PtrFromString(deviceName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		windows.Handle(0))
	if err != nil {
		return nil, err
	}
	return &Handle{Handle: h}, nil
}

func decodeStruct(data []uint8, sz uint32) (pid uint64, imagefile, cmdline string, consumed uint32) {
	n := *(*DDProcessNotification)(unsafe.Pointer(&data[0]))

	consumed = uint32(n.Size)
	pid = uint64(n.ProcessId)

	if n.NotifyType == ProcmonNotifyStart {
		imagefile = winutil.ConvertWindowsString(data[n.ImageFileOffset : n.ImageFileOffset+n.ImageFileLen])
		cmdline = winutil.ConvertWindowsString(data[n.CommandLineOffset : n.CommandLineOffset+n.CommandLineLen])
	}
	return
}
