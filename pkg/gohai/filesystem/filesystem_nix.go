// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build darwin || linux

package filesystem

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"

	log "github.com/cihub/seelog"
	"github.com/moby/sys/mountinfo"
)

// fsInfoGetter provides function to get information about a given filesystem: its size and its dev id
type fsInfoGetter interface {
	// SizeKB returns the size of the given filesystem in KB
	SizeKB(mount *mountinfo.Info) (uint64, error)

	// Dev returns the dev id of the given filesystem
	Dev(mount *mountinfo.Info) (uint64, error)
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

func (unixFSInfo) Dev(mount *mountinfo.Info) (uint64, error) {
	var stat unix.Stat_t
	if err := unix.Stat(mount.Mountpoint, &stat); err != nil {
		return 0, fmt.Errorf("stat %s: %w", mount.Source, err)
	}

	return uint64(stat.Dev), nil
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

	devMountInfos := map[uint64]MountInfo{}
	for _, mount := range mounts {
		// Skip mounts that seem to be missing data
		if mount.Source == "" || mount.Source == "none" || mount.FSType == "" || mount.Mountpoint == "" {
			continue
		}

		if isDummyFS(mount.FSType) {
			continue
		}

		if isRemoteFS(mount.FSType, mount.Source) {
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

// isDummyFS returns whether to ignore the filesystem type
func isDummyFS(fsType string) bool {
	// hardcoded list of types to ignore, from gnulib implementation
	// https://github.com/coreutils/gnulib/blob/fc3c64b0a0e0acffd6de1e76fa23b787fc8e931b/lib/mountlist.c#L171-L201
	ignoredFSTypes := map[string]struct{}{
		"autofs":      {},
		"debugfs":     {},
		"devfs":       {},
		"devpts":      {},
		"devtmpfs":    {}, // added by a patch on ubuntu
		"fuse.portal": {},
		"fusectl":     {},
		"ignore":      {},
		"kernfs":      {},
		"mqueue":      {},
		"none":        {},
		"proc":        {},
		"rpc_pipefs":  {},
		"squashfs":    {}, // added by a patch on ubuntu
		"subfs":       {},
		"sysfs":       {},
	}

	_, found := ignoredFSTypes[fsType]
	return found
}

// isRemoteFS returns whether a filesystem with the given type and source is remote.
//
// It is considered remote if
// - the source contains a ':'
// - the source is "-hosts"
// - the source starts with "//" and the type is "smbfs", "smb3" or "cifs"
// - the type is known to be remote
//
// The logic comes from gnulib
// https://github.com/coreutils/gnulib/blob/fc3c64b0a0e0acffd6de1e76fa23b787fc8e931b/lib/mountlist.c#L231-L254
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

	// list of known remote filesystem types
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

	_, found := remoteFSTypes[fsType]
	return found
}
