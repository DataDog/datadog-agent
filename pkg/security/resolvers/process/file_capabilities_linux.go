// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/binary"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	vfsCapRevisionMask = 0xff000000
	vfsCapRevision1    = 0x01000000
	vfsCapRevision2    = 0x02000000
	vfsCapRevision3    = 0x03000000
	vfsCapEffective    = 0x00000001

	vfsCapSize1 = 12
	vfsCapSize2 = 20
	vfsCapSize3 = 24
)

// readFileCapabilitiesFromProcfs reads the current executable's xattr, not the
// process credentials or the capabilities it had at exec time. Opening the procfs
// exe link, rather than its readlink target, also works for deleted/replaced
// executables and for processes in a different mount namespace. Pin the file
// while reading and check its inode against the metadata already collected.
// Missing attributes, inaccessible files and unsupported formats leave capabilities
// unavailable without preventing process resolution.
func readFileCapabilitiesFromProcfs(execPath string, inode uint64) model.FileCapabilities {
	if inode == 0 {
		return model.FileCapabilities{}
	}

	fd, err := unix.Open(execPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return model.FileCapabilities{}
	}
	defer unix.Close(fd)

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Ino != inode {
		return model.FileCapabilities{}
	}

	var data [vfsCapSize3]byte
	n, err := unix.Fgetxattr(fd, "security.capability", data[:])
	if err != nil {
		return model.FileCapabilities{}
	}
	return parseFileCapabilities(data[:n])
}

// parseFileCapabilities decodes the little-endian on-disk vfs_cap_data format,
// which is different from both cpu_vfs_cap_data and our eBPF event representation.
func parseFileCapabilities(data []byte) model.FileCapabilities {
	if len(data) < 4 {
		return model.FileCapabilities{}
	}

	magic := binary.LittleEndian.Uint32(data[:4])
	revision := magic & vfsCapRevisionMask
	var size int
	switch revision {
	case vfsCapRevision1:
		size = vfsCapSize1
	case vfsCapRevision2:
		size = vfsCapSize2
	case vfsCapRevision3:
		size = vfsCapSize3
	default:
		return model.FileCapabilities{}
	}
	if len(data) != size || magic & ^uint32(vfsCapRevisionMask|vfsCapEffective) != 0 {
		return model.FileCapabilities{}
	}

	caps := model.FileCapabilities{
		Permitted:   uint64(binary.LittleEndian.Uint32(data[4:8])),
		Inheritable: uint64(binary.LittleEndian.Uint32(data[8:12])),
		Effective:   magic&vfsCapEffective != 0,
		Available:   true,
	}
	if revision != vfsCapRevision1 {
		caps.Permitted |= uint64(binary.LittleEndian.Uint32(data[12:16])) << 32
		caps.Inheritable |= uint64(binary.LittleEndian.Uint32(data[16:20])) << 32
	}
	// Revision 3 also carries a rootid. The file sets themselves are reported
	// here, without claiming they apply to this process's user namespace.
	return caps
}
