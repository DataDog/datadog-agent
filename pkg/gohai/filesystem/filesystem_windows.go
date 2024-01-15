// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package filesystem

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// InvalidHandle is the value returned in case of error
const InvalidHandle windows.Handle = ^windows.Handle(0)

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
	var mod = windows.NewLazyDLL("kernel32.dll")
	var getDisk = mod.NewProc("GetDiskFreeSpaceExW")
	var sz uint64
	var fr uint64

	volWinStr, err := windows.UTF16PtrFromString(vol)
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
	var mod = windows.NewLazyDLL("kernel32.dll")
	var getPaths = mod.NewProc("GetVolumePathNamesForVolumeNameW")
	var tmp uint32
	var objlistsize uint32 = 0x0
	var retval []string

	volWinStr, err := windows.UTF16PtrFromString(vol)
	if err != nil {
		return retval
	}
	status, _, errno := getPaths.Call(uintptr(unsafe.Pointer(volWinStr)),
		uintptr(unsafe.Pointer(&tmp)),
		2,
		uintptr(unsafe.Pointer(&objlistsize)))

	if status != 0 || errno != windows.ERROR_MORE_DATA {
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

func getFileSystemInfo() ([]MountInfo, error) {
	var mod = windows.NewLazyDLL("kernel32.dll")
	var findFirst = mod.NewProc("FindFirstVolumeW")
	var findNext = mod.NewProc("FindNextVolumeW")
	var findClose = mod.NewProc("FindVolumeClose")

	buf := make([]uint16, 512)
	var sz int32 = 512
	fh, _, _ := findFirst.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(sz))
	var findHandle = windows.Handle(fh)
	var fileSystemInfo []MountInfo

	if findHandle != InvalidHandle {
		// ignore close error
		//nolint:errcheck
		defer findClose.Call(fh)
		moreData := true
		for moreData {
			outstring := convertWindowsString(buf)

			size, _ := getDiskSize(outstring)

			mountpts := getMountPoints(outstring)
			var mountName string
			if len(mountpts) > 0 {
				mountName = mountpts[0]
			}
			mountInfo := MountInfo{
				Name:      outstring,
				SizeKB:    size / 1024,
				MountedOn: mountName,
			}
			fileSystemInfo = append(fileSystemInfo, mountInfo)
			status, _, _ := findNext.Call(fh,
				uintptr(unsafe.Pointer(&buf[0])),
				uintptr(size))
			if 0 == status {
				moreData = false
			}
		}

	}

	return fileSystemInfo, nil
}
