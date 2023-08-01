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

// ignoredFSTypes are filesystem types to ignore
var ignoredFSTypes = map[string]struct{}{
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

// remoteFSTypes are filesystem types known to be remote
var remoteFSTypes = map[string]struct{}{
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

// fsInfoGetter provides function to get information about a given filesystem: its size and its dev id
type fsInfoGetter interface {
	// SizeKB returns the size of the given filesystem in KB
	SizeKB(mount *mountinfo.Info) (uint64, error)

	// Dev returns the dev id of the given filesystem
	// the return type is interface{} because `syscall.Stat_t` uses different types for Dev depending on the platform
	Dev(mount *mountinfo.Info) (interface{}, error)
}

type unixFSInfo struct{}

func (unixFSInfo) SizeKB(mount *mountinfo.Info) (uint64, error) {
	var statfs unix.Statfs_t
	if err := unix.Statfs(mount.Mountpoint, &statfs); err != nil {
		return 0, fmt.Errorf("statfs %s: %v", mount.Source, err)
	}

	sizeKB := statfs.Blocks * uint64(statfs.Bsize) / 1024
	return sizeKB, nil
}

func (unixFSInfo) Dev(mount *mountinfo.Info) (interface{}, error) {
	var stat unix.Stat_t
	if err := unix.Stat(mount.Mountpoint, &stat); err != nil {
		return 0, fmt.Errorf("stat %s: %w", mount.Source, err)
	}

	return stat.Dev, nil
}

func getFileSystemInfo() ([]MountInfo, error) {
	mounts, err := mountinfo.GetMounts(nil)
	if err != nil {
		return nil, err
	}

	return getFileSystemInfoWithMounts(mounts, unixFSInfo{})
}

// replaceDev returns whether to use the new mountInfo instead of the old one.
//
// The same filesystem can appear several times in the list of filesystems, for example when it
// has several mount points.
// In order to have each filesystem appear only once in the final list, we get the "dev id" of the
// fs using the stat syscall, then keep only one entry per id with the following logic:
// - prefer fs whose name contains a /
// - prefer fs with shorter mount path ("closer to the root")
// - prefer the most recent one (ie. the new one)
//
// This behavior is inspired by what df does.
func replaceDev(old, new MountInfo) bool {
	if strings.ContainsRune(new.Name, '/') && !strings.ContainsRune(old.Name, '/') {
		return true
	}
	if len(old.MountedOn) > len(new.MountedOn) {
		return true
	}

	return old.Name != new.Name && old.MountedOn == new.MountedOn
}

// getFileSystemInfoWithMounts is an internal method to help testing with test mounts
func getFileSystemInfoWithMounts(initialMounts []*mountinfo.Info, fsInfo fsInfoGetter) ([]MountInfo, error) {
	mounts := initialMounts

	devMountInfos := map[interface{}]MountInfo{}
	for _, mount := range mounts {
		// Skip mounts that seem to be missing data
		if mount.Source == "" || mount.Source == "none" || mount.FSType == "" || mount.Mountpoint == "" {
			continue
		}

		if isExcludedFS(mount.FSType, mount.Source, true) {
			continue
		}

		sizeKB, err := fsInfo.SizeKB(mount)
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

// isExcludedFS returns whether to ignore the given filesystem type and source.
//
// It is ignored if
// - the type is in ignoredFSTypes
// - we only want local filesystems but it is detected as remote
func isExcludedFS(fsType string, fsSource string, localOnly bool) bool {
	// Some filesystems should be ignored based on type
	if _, ok := ignoredFSTypes[fsType]; ok {
		return true
	}

	if localOnly && isRemoteFS(fsType, fsSource) {
		return true
	}

	return false
}

// isRemoteFS returns whether a filesystem with the given type and source is remote.
//
// It is considered remote if
// - the source contains a ':'
// - the source is "-hosts"
// - the source starts with "//" and the type is "smbfs", "smb3" or "cifs"
// - the type is in remoteFSTypes
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
	_, found := remoteFSTypes[fsType]
	return found
}
