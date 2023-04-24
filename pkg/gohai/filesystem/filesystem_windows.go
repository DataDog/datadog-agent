// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package filesystem

import (
	"strconv"
	"syscall"
	"unsafe"
)

// Handle represents a pointer used by FindFirstVolumeW and similar functions
type Handle uintptr

// InvalidHandle is the value returned in case of error
const InvalidHandle Handle = ^Handle(0)

// ERRORMoreData is the error returned when the size is not big enough
const ERRORMoreData syscall.Errno = 234

// this would probably go in a common utilities rather than here

func convertWindowsStringList(winput []uint16) []string {
	var retstrings []string
	var rsindex = 0

	retstrings = append(retstrings, "")
	for i := 0; i < (len(winput) - 1); i++ {
		if winput[i] == 0 {
			if winput[i+1] == 0 {
				return retstrings
			}
			rsindex++
			retstrings = append(retstrings, "")
			continue
		}
		retstrings[rsindex] += string(rune(winput[i]))
	}
	return retstrings
}

// as would this
func convertWindowsString(winput []uint16) string {
	var retstring string
	for i := 0; i < len(winput); i++ {
		if winput[i] == 0 {
			break
		}
		retstring += string(rune(winput[i]))
	}
	return retstring
}

func getDiskSize(vol string) (size uint64, freespace uint64) {
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var getDisk = mod.NewProc("GetDiskFreeSpaceExW")
	var sz uint64
	var fr uint64

	volWinStr, err := syscall.UTF16PtrFromString(vol)
	if err != nil {
		return 0, 0
	}
	status, _, _ := getDisk.Call(uintptr(unsafe.Pointer(volWinStr)),
		uintptr(0),
		uintptr(unsafe.Pointer(&sz)),
		uintptr(unsafe.Pointer(&fr)))
	if status == 0 {
		return 0, 0
	}
	return sz, fr
}

func getMountPoints(vol string) []string {
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var getPaths = mod.NewProc("GetVolumePathNamesForVolumeNameW")
	var tmp uint32
	var objlistsize uint32 = 0x0
	var retval []string

	volWinStr, err := syscall.UTF16PtrFromString(vol)
	if err != nil {
		return retval
	}
	status, _, errno := getPaths.Call(uintptr(unsafe.Pointer(volWinStr)),
		uintptr(unsafe.Pointer(&tmp)),
		2,
		uintptr(unsafe.Pointer(&objlistsize)))

	if status != 0 || errno != ERRORMoreData {
		// unexpected
		return retval
	}

	buf := make([]uint16, objlistsize)
	status, _, _ = getPaths.Call(uintptr(unsafe.Pointer(volWinStr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(objlistsize),
		uintptr(unsafe.Pointer(&objlistsize)))
	if status == 0 {
		return retval
	}
	return convertWindowsStringList(buf)

}

func getFileSystemInfo() (interface{}, error) {
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var findFirst = mod.NewProc("FindFirstVolumeW")
	var findNext = mod.NewProc("FindNextVolumeW")
	var findClose = mod.NewProc("FindVolumeClose")

	//var findHandle Handle
	buf := make([]uint16, 512)
	var sz int32 = 512
	fh, _, _ := findFirst.Call(uintptr(unsafe.Pointer(&buf[0])),
		uintptr(sz))
	var findHandle = Handle(fh)
	var fileSystemInfo []interface{}

	if findHandle != InvalidHandle {
		// ignore close error
		//nolint:errcheck
		defer findClose.Call(fh)
		moreData := true
		for moreData {
			outstring := convertWindowsString(buf)
			sz, _ := getDiskSize(outstring)
			var capacity string
			if 0 == sz {
				capacity = "Unknown"
			} else {
				capacity = strconv.FormatInt(int64(sz)/1024.0, 10)
			}
			mountpts := getMountPoints(outstring)
			var mountName string
			if len(mountpts) > 0 {
				mountName = mountpts[0]
			}
			iface := map[string]interface{}{
				"name":       outstring,
				"kb_size":    capacity,
				"mounted_on": mountName,
			}
			fileSystemInfo = append(fileSystemInfo, iface)
			status, _, _ := findNext.Call(fh,
				uintptr(unsafe.Pointer(&buf[0])),
				uintptr(sz))
			if 0 == status {
				moreData = false
			}
		}

	}

	return fileSystemInfo, nil
}
