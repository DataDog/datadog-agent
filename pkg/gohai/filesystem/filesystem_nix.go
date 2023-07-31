// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build darwin || linux
// +build darwin linux

package filesystem

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"

	log "github.com/cihub/seelog"
	"github.com/moby/sys/mountinfo"
)

// These FS types should be excluded from listing
var IgnoredFSTypes = map[string]struct{}{
	"autofs":      {},
	"debugfs":     {},
	"devfs":       {},
	"devpts":      {},
	"devtmpfs":    {},
	"fuse.portal": {},
	"fusectl":     {},
	"ignore":      {},
	"kernfs":      {},
	"mqueue":      {},
	"none":        {},
	"proc":        {},
	"rpc_pipefs":  {},
	"squashfs":    {},
	"subfs":       {},
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

// FSInfoGetter provides function to get information about a given filesystem: its size and its dev id
type FSInfoGetter interface {
	// Size returns the size of the given filesystem
	Size(mount *mountinfo.Info) (uint64, error)

	// Dev returns the dev id of the given filesystem
	// the return type is interface{} because `syscall.Stat_t` uses different types for Dev depending on the platform
	Dev(mount *mountinfo.Info) (interface{}, error)
}

type UnixFSInfo struct{}

func (UnixFSInfo) Size(mount *mountinfo.Info) (uint64, error) {
	var statfs unix.Statfs_t
	if err := unix.Statfs(mount.Mountpoint, &statfs); err != nil {
		return 0, fmt.Errorf("statfs %s: %v", mount.Source, err)
	}

	sizeKB := statfs.Blocks * uint64(statfs.Bsize) / 1024
	return sizeKB, nil
}

func (UnixFSInfo) Dev(mount *mountinfo.Info) (interface{}, error) {
	var stat unix.Stat_t
	if err := unix.Stat(mount.Mountpoint, &stat); err != nil {
		return 0, fmt.Errorf("stat %s: %w", mount.Source, err)
	}

	return stat.Dev, nil
}

func getFileSystemInfo() ([]MountInfo, error) {
	return getFileSystemInfoWithMounts(nil, UnixFSInfo{})
}

// returns whether to use the new mountInfo instead of the old one
func replaceDev(old, new MountInfo) bool {
	if strings.ContainsRune(new.Name, '/') && !strings.ContainsRune(old.Name, '/') {
		return true
	}
	if len(old.MountedOn) > len(new.MountedOn) {
		return true
	}

	return old.Name != new.Name && old.MountedOn == new.MountedOn
}

// Internal method to help testing with test mounts
func getFileSystemInfoWithMounts(initialMounts []*mountinfo.Info, fsInfo FSInfoGetter) ([]MountInfo, error) {
	var err error
	mounts := initialMounts

	if mounts == nil {
		mounts, err = mountinfo.GetMounts(nil)
		if err != nil {
			return nil, err
		}
	}

	devMountInfos := map[interface{}]MountInfo{}
	for _, mount := range mounts {
		// Skip mounts that seem to be missing data
		if mount.Source == "" || mount.Source == "none" || mount.FSType == "" || mount.Mountpoint == "" {
			continue
		}

		if isExcludedFS(mount.FSType, mount.Source, true) {
			continue
		}

		sizeKB, err := fsInfo.Size(mount)
		if err != nil {
			log.Info(err)
			continue
		}

		if sizeKB == 0 {
			// ignore zero-sized filesystems, like `df` does (unless using -a)
			continue
		}

		mountInfo := MountInfo{
			Name:      mount.Source,
			SizeKB:    sizeKB,
			MountedOn: mount.Mountpoint,
		}

		dev, err := fsInfo.Dev(mount)
		if err != nil {
			log.Info(err)
			continue
		}

		existingMountInfo, exists := devMountInfos[dev]
		if !exists || replaceDev(existingMountInfo, mountInfo) {
			devMountInfos[dev] = mountInfo
		}
	}

	mountInfos := make([]MountInfo, 0, len(devMountInfos))
	for _, mountInfo := range devMountInfos {
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
