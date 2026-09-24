// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func getInodeNumFromLink(link string) (uint64, error) {
	start := strings.LastIndexByte(link, '[')
	end := strings.LastIndexByte(link, ']')
	if start == -1 || end == -1 || start >= end-1 {
		return 0, fmt.Errorf("invalid link: %s", link)
	}
	inodeStr := link[start+1 : end]
	ino, err := strconv.ParseUint(inodeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid link: %s", link)
	}
	return ino, nil
}

// relativizeSnapshotMountPoints makes the absolute mount points of a mount namespace snapshot relative to the
// mount point of their parent, like the mount points from kernel events, so that the path of a mount can be
// computed again once one of its ancestors moved
func relativizeSnapshotMountPoints(mnts []*model.Mount) {
	paths := make(map[uint32]string, len(mnts))
	for _, m := range mnts {
		paths[m.MountID] = m.Path
	}

	for _, m := range mnts {
		parentPath, ok := paths[m.ParentPathKey.MountID]
		if !ok || m.ParentPathKey.MountID == m.MountID {
			continue
		}

		switch {
		case m.Path == parentPath:
			// "/" would be taken as the root of the namespace
			m.MountPointStr = "."
		case parentPath == "/":
			m.MountPointStr = strings.TrimPrefix(m.Path, "/")
		default:
			if rel, found := strings.CutPrefix(m.Path, parentPath+"/"); found {
				m.MountPointStr = rel
			}
		}
	}
}

// trimMountRoot makes a path relative to the file system root, relative to a mount root instead
func trimMountRoot(p string, root string) string {
	if root == "" || root == "/" {
		return p
	}

	absPath := "/" + strings.TrimPrefix(p, "/")
	if absPath == root {
		return "/"
	}
	if rel, found := strings.CutPrefix(absPath, root+"/"); found {
		return rel
	}
	return p
}
