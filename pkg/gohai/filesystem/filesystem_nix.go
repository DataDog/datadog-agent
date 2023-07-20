// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build darwin || linux
// +build darwin linux

package filesystem

import (
	"strings"
	"syscall"

	"github.com/moby/sys/mountinfo"
)

// These FS types should be excluded from listing
var IgnoredFSTypes = map[string]struct{}{
	"autofs":      {},
	"debugfs":     {},
	"devfs":       {},
	"devpts":      {},
	"fuse.portal": {},
	"fusectl":     {},
	"ignore":      {},
	"kernfs":      {},
	"none":        {},
	"proc":        {},
	"subfs":       {},
	"mqueue":      {},
	"rpc_pipefs":  {},
	"sysfs":       {},
}

// These FS types are known to be remote
var RemoteFSTypes = map[string]struct{}{
	"acfs":       {},
	"afs":        {},
	"auristorfs": {},
	"coda":       {},
	"fhgfs":      {},
	"gpfs":       {},
	"ibrix":      {},
	"ocfs2":      {},
	"vxfs":       {},
}

func getFileSystemInfo() ([]MountInfo, error) {
	return getFileSystemInfoWithMounts(nil)
}

// Internal method to help testing with test mounts
func getFileSystemInfoWithMounts(initialMounts []*mountinfo.Info) ([]MountInfo, error) {
	var err error
	mounts := initialMounts

	if mounts == nil {
		mounts, err = mountinfo.GetMounts(nil)
		if err != nil {
			return nil, err
		}
	}

	var mountInfos = make([]MountInfo, 0, len(mounts))

	for _, mount := range mounts {
		if isExcludedFS(mount.FSType, mount.Source, true) {
			continue
		}

		var stat syscall.Statfs_t

		sizeKB := uint64(0)
		if err := syscall.Statfs(mount.Mountpoint, &stat); err == nil {
			sizeKB = (stat.Blocks * uint64(stat.Bsize)) / 1024
		}

		if sizeKB == 0 {
			// Skip empty filesystems, like `df` does (unless using -a)
			continue
		}

		// Skip mounts that seem to be missing data
		if mount.Source == "" || mount.Source == "none" || mount.FSType == "" || mount.Mountpoint == "" {
			continue
		}

		mountInfo := MountInfo{
			Name:      mount.Source,
			SizeKB:    sizeKB,
			MountedOn: mount.Mountpoint,
		}
		mountInfos = append(mountInfos, mountInfo)
	}

	return mountInfos, nil
}

func isExcludedFS(fsType string, fsSource string, localOnly bool) bool {
	// Some filesystems should be ignored based on type
	if _, ok := IgnoredFSTypes[fsType]; ok {
		return true
	}

	if localOnly && isRemoteFS(fsType, fsSource) {
		return true
	}

	return false
}

func isRemoteFS(fsType string, fsSource string) bool {
	// If we have a `:` in the source, it should be remote
	if strings.Contains(fsSource, ":") {
		return true
	}

	// This is a special name for remote mounts
	if fsSource == "-hosts" {
		return true
	}

	// If we start with `//` and we're one of the listed FS types, it's
	// a remote mount
	if len(fsSource) > 2 && fsSource[0:2] == "//" {
		switch fsType {
		case "smbfs", "smb3", "cifs":
			return true
		}
	}

	// Check for general FS types that are known to be remote
	_, found := RemoteFSTypes[fsType]
	return found
}
