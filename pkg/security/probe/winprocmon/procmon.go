//go:build windows
// +build windows

package winprocmon

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Handle struct {
	windows.Handle
}

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddprocmon`
)

type WinProcessNotification struct {
	Type      uint64 // will be ProcmonNotifyStop or ProcmonNotifyStart
	Pid       uint64
	ImageFile string
	CmdLine   string
}

// NewHandle creates a new windows handle attached to the driver
func NewHandle(flags uint32) (*Handle, error) {
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

func RunLoop(f func(*WinProcessNotification)) {
	h, err := NewHandle(0)
	if err != nil {
		fmt.Printf("Error creating handle %v\n", err)
		return
	}
	readbuffer := make([]uint8, 1024)
	for {
		found := false
		var returnedsize uint32
		err := windows.DeviceIoControl(h.Handle,
			GetNewProcsIOCTL,
			nil,
			0,
			(*byte)(unsafe.Pointer(&readbuffer[0])),
			1024,
			&returnedsize,
			nil)
		if err == nil && returnedsize > 0 {
			consumed := uint32(0)
			for consumed < returnedsize {
				wpn, used := decodeStruct(readbuffer[consumed:], returnedsize-consumed)
				consumed += used
				f(wpn)
			}
			found = true
		}
		err = windows.DeviceIoControl(h.Handle,
			GetDeadProcsIOCTL,
			nil,
			0,
			(*byte)(unsafe.Pointer(&readbuffer[0])),
			1024,
			&returnedsize,
			nil)
		if err == nil && returnedsize > 0 {
			consumed := uint32(0)
			for consumed < returnedsize {
				wpn, used := decodeStruct(readbuffer[consumed:], returnedsize-consumed)
				consumed += used
				f(wpn)
			}
			found = true
		}
		if !found {
			time.Sleep(2 * time.Second)
		}

	}
}

func convertWindowsString(winput []uint8) string {

	p := (*[1 << 29]uint16)(unsafe.Pointer(&winput[0]))[: len(winput)/2 : len(winput)/2]
	return windows.UTF16ToString(p)

}
func decodeStruct(data []uint8, sz uint32) (*WinProcessNotification, uint32) {
	n := *(*DDProcessNotification)(unsafe.Pointer(&data[0]))

	wpn := &WinProcessNotification{}

	wpn.Type = n.NotifyType

	consumed := uint32(n.Size)
	wpn.Pid = n.ProcessId

	if n.NotifyType == ProcmonNotifyStart {
		wpn.ImageFile = convertWindowsString(data[n.ImageFileOffset : n.ImageFileOffset+n.ImageFileLen])
		wpn.CmdLine = convertWindowsString(data[n.CommandLineOffset : n.CommandLineOffset+n.CommandLineLen])
	}
	return wpn, consumed
}

/*
func printProcs(data []uint8, sz uint32) {
	var consumed uint32
	for consumed < sz {
		pid, img, cmd, used := decodeStruct(data[consumed:], sz-consumed)
		consumed += used
		fmt.Printf("Pid %v  image %v   cmd %v\n", pid, img, cmd)
	}
}

func printDeadProcs(data []uint8, sz uint32) {
	fmt.Printf("\n Dead Procs: \n")
	fmt.Printf("\n ------------------------- \n")
	printProcs(data, sz)
}

func printNewProcs(data []uint8, sz uint32) {
	fmt.Printf("\n New Procs: \n")
	fmt.Printf("\n ------------------------- \n")
	printProcs(data, sz)
}
*/
