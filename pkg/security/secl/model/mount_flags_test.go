// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

// TestMountFlagsNormalizationEquivalence is the anti-duplication guarantee: the
// same logical mount observed through any source must normalize to the same
// canonical bitmask, otherwise the profile mount table would key it differently
// and duplicate.
func TestMountFlagsNormalizationEquivalence(t *testing.T) {
	const (
		rawMntReadOnly    = 0x40
		rawMntNoSUID      = 0x01
		rawMntNoDev       = 0x02
		rawMntNoExec      = 0x04
		rawMntNoSymFollow = 0x80
	)

	tests := []struct {
		name     string
		vfs      uint32
		attr     uint64
		options  string
		expected uint32
	}{
		{
			name:     "read-only noexec",
			vfs:      rawMntReadOnly | rawMntNoExec,
			attr:     unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NOEXEC,
			options:  "ro,noexec,relatime",
			expected: MountAttrReadOnly | MountAttrNoExec,
		},
		{
			name:     "all security flags",
			vfs:      rawMntReadOnly | rawMntNoSUID | rawMntNoDev | rawMntNoExec | rawMntNoSymFollow,
			attr:     unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NODEV | unix.MOUNT_ATTR_NOEXEC | unix.MOUNT_ATTR_NOSYMFOLLOW,
			options:  "ro,nosuid,nodev,noexec,nosymfollow",
			expected: MountAttrReadOnly | MountAttrNoSUID | MountAttrNoDev | MountAttrNoExec | MountAttrNoSymFollow,
		},
		{
			name:     "writable executable",
			vfs:      0,
			attr:     0,
			options:  "rw,relatime",
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, NormalizeMountFlagsFromVFS(tc.vfs), "vfs")
			assert.Equal(t, tc.expected, NormalizeMountFlagsFromAttr(tc.attr), "attr")
			assert.Equal(t, tc.expected, NormalizeMountFlagsFromOptions(tc.options), "options")
		})
	}
}

func TestNormalizeMountFlagsFromAttrMasksUnknownBits(t *testing.T) {
	// atime bits and other non-security attributes must not leak into the canonical value
	attr := uint64(unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NOATIME | unix.MOUNT_ATTR_NODIRATIME)
	assert.Equal(t, MountAttrReadOnly, NormalizeMountFlagsFromAttr(attr))
}
