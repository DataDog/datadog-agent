// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package winutil

import (
	"fmt"
	"regexp"
	"syscall"

	"golang.org/x/sys/windows"
)

var driveRegexp = regexp.MustCompile("(?i)[a-z]:\\\\")

// GetDriveFsType returns the filesystem name of a drive (ex: "C:\")
func GetDriveFsType(driveName string) (string, error) {
	if !driveRegexp.MatchString(driveName) {
		return "", fmt.Errorf("driveName is not a correct drive name: %s", driveName)
	}
	maxLength := uint32(syscall.MAX_PATH + 1)
	volumeNameBuffer := make([]uint16, maxLength)
	maximumComponentLength := uint32(0)
	fileSystemFlags := uint32(0)
	fileSystemNameBuffer := make([]uint16, maxLength)
	windows.GetVolumeInformation(
		syscall.StringToUTF16Ptr(driveName),
		&volumeNameBuffer[0],
		maxLength,
		nil,
		&maximumComponentLength,
		&fileSystemFlags,
		&fileSystemNameBuffer[0],
		maxLength,
	)

	return syscall.UTF16ToString(fileSystemNameBuffer), nil

}
