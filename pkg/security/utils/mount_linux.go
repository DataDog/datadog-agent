// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"golang.org/x/sys/unix"
)

type magicFS struct {
	magic int64
	fs    string
}

// copied from /usr/include/linux/magic.h
var magic2fs = []magicFS{
	{0xadf5, "adfs"},
	{0xadff, "affs"},
	{0x5346414, "afsf"},
	{0x0187, "autofs"},
	{0x73757245, "coda"},
	{0x28cd3d45, "cramfs"},
	{0x453dcd28, "cramfs_wend"},
	{0x64626720, "debugfs"},
	{0x73636673, "securityfs"},
	{0xf97cff8c, "selinux"},
	{0x43415d53, "smack"},
	{0x858458f6, "ramfs"},
	{0x01021994, "tmpfs"},
	{0x958458f6, "hugetlbfs"},
	{0x73717368, "squashfs"},
	{0xf15f, "ecryptfs"},
	{0x414a53, "efs"},
	{0xe0f5e1e2, "erofs_v1"},
	{0xef53, "ext2"},
	{0xef53, "ext3"},
	{0xabba1974, "xenfs"},
	{0xef53, "ext4"},
	{0x9123683e, "btrfs"},
	{0x3434, "nilfs"},
	{0xf2f52010, "f2fs"},
	{0xf995e849, "hpfs"},
	{0x9660, "isofs"},
	{0x72b6, "jffs2"},
	{0x58465342, "xfs"},
	{0x6165676c, "pstorefs"},
	{0xde5e81e4, "efivarfs"},
	{0x00c0ffee, "hostfs"},
	{0x794c7630, "overlayfs"},
	{0x137f, "minix"},
	{0x138f, "minix2"},
	{0x2468, "minix2"},
	{0x2478, "minix22"},
	{0x4d5a, "minix3"},
	{0x4d44, "msdos"},
	{0x564c, "ncp"},
	{0x6969, "nfs"},
	{0x7461636f, "ocfs2"},
	{0x9fa1, "openprom"},
	{0x002f, "qnx4"},
	{0x68191122, "qnx6"},
	{0x6b414653, "afs_fs"},
	{0x52654973, "reiserfs"},
	{0x517b, "smb"},
	{0x27e0eb, "cgroup"},
	{0x63677270, "cgroup2"},
	{0x7655821, "rdtgroup"},
	{0x57ac6e9d, "stack_end"},
	{0x74726163, "tracefs"},
	{0x01021997, "v9fs"},
	{0x62646576, "bdevfs"},
	{0x64646178, "daxfs"},
	{0x42494e4d, "binfmtfs"},
	{0x1cd1, "devpts"},
	{0x6c6f6f70, "binderfs"},
	{0xbad1dea, "futexfs"},
	{0x50495045, "pipefs"},
	{0x9fa0, "proc"},
	{0x534f434b, "sockfs"},
	{0x62656572, "sysfs"},
	{0x9fa2, "usbdevice"},
	{0x11307854, "mtd_inode_fs"},
	{0x09041934, "anon_inode_fs"},
	{0x73727279, "btrfs_test"},
	{0x6e736673, "nsfs"},
	{0xcafe4a11, "bpf_fs"},
	{0x5a3c69f0, "aafs"},
	{0x15013346, "udf"},
	{0x13661366, "balloon_kvm"},
	{0x58295829, "zsmalloc"},
	{0x444d4142, "dma_buf"},
	{0x454d444d, "devmem"},
	{0x33, "z3fold"},
	{0x6a656a62, "shiftfs"},
}

// GetFSTypeFromFilePath returns the filesystem type of the mount holding the speficied file path
func GetFSTypeFromFilePath(path string) string {
	var s unix.Statfs_t
	err := unix.Statfs(path, &s)
	if err != nil {
		return ""
	}
	for _, m := range magic2fs {
		if m.magic == s.Type {
			return m.fs
		}
	}
	return ""
}
