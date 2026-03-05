// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package filesystem

import (
	"os/exec"
	"strconv"
	"strings"
)

func getFileSystemInfo() ([]MountInfo, error) {
	// df -k reports 1024-byte block counts; output format:
	// Filesystem    1024-blocks      Free %Used    Iused %Iused Mounted on
	// /dev/hd4           131072     91312   31%     7011     5% /
	out, err := exec.Command("df", "-k").Output()
	if err != nil {
		return nil, err
	}

	var mounts []MountInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		device := fields[0]
		// Skip pseudo/remote filesystems
		if device == "-" || device == "none" || strings.Contains(device, ":") {
			continue
		}
		// Skip if size field is "-" (e.g. /proc on AIX)
		if fields[1] == "-" {
			continue
		}
		sizeKB, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil || sizeKB == 0 {
			continue
		}
		mountedOn := fields[6]
		mounts = append(mounts, MountInfo{
			Name:      device,
			SizeKB:    sizeKB,
			MountedOn: mountedOn,
		})
	}
	return mounts, nil
}
