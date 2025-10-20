// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/moby/sys/mountinfo"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// newMountFromMountInfo - Creates a new Mount from parsed MountInfo data
func newMountFromMountInfo(mnt *mountinfo.Info) *model.Mount {
	root := mnt.Root

	if mnt.FSType == "btrfs" {
		var subvol string
		for _, opt := range strings.Split(mnt.VFSOptions, ",") {
			name, val, ok := strings.Cut(opt, "=")
			if ok && name == "subvol" {
				subvol = val
			}
		}

		if subvol != "" {
			root = strings.TrimPrefix(root, subvol)
		}

		if root == "" {
			root = "/"
		}
	}

	if mnt.FSType == "cgroup2" && strings.HasPrefix(root, "/..") {
		cfs := utils.DefaultCGroupFS()
		root = filepath.Join(cfs.GetRootCGroupPath(), root)
	}

	// create a Mount out of the parsed MountInfo
	return &model.Mount{
		MountID: uint32(mnt.ID),
		Device:  utils.Mkdev(uint32(mnt.Major), uint32(mnt.Minor)),
		ParentPathKey: model.PathKey{
			MountID: uint32(mnt.Parent),
		},
		FSType:        mnt.FSType,
		MountPointStr: mnt.Mountpoint,
		Path:          mnt.Mountpoint,
		RootStr:       root,
		Origin:        model.MountOriginProcfs,
		Visible:       true,
		Detached:      false,
	}
}

func GetAllProcfs(procfs string, cb func(*model.Mount)) error {
	seen := make(map[uint64]struct{})

	pids, err := os.ReadDir(procfs)
	if err != nil {
		return err
	}
	for _, p := range pids {
		if !p.IsDir() {
			continue
		}
		pid := p.Name()
		pidInt, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		p := filepath.Join(procfs, pid, "ns", "mnt")
		linkTarget, err := os.Readlink(p)
		if err != nil {
			continue
		}

		ino, err := getInodeNumFromLink(linkTarget)
		if err != nil {
			continue
		}

		if _, ok := seen[ino]; ok {
			continue
		}

		mnts, err := kernel.ParseMountInfoFile(int32(pidInt))
		if err != nil {
			// TODO: Report error here??
			continue
		}

		for _, m := range mnts {
			mnt := newMountFromMountInfo(m)
			mnt.NamespaceInode = ino
			cb(mnt)
		}

		seen[ino] = struct{}{}
	}
	return nil
}
