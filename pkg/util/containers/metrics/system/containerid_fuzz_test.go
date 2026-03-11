// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"os"
	"testing"
)

// FuzzParseMountinfo exercises parseMountinfo against fuzzer-controlled
// /proc/self/mountinfo content. The guard on matches is len(matches) > 0
// (at least 1 element) while the code then accesses indices 1 and 2; fuzz
// to confirm no edge-case line can produce a partial regex match.
func FuzzParseMountinfo(f *testing.F) {
	// Normal Docker cgroupv2 line
	f.Add([]byte("557 530 0:148 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:81 - cgroup2 cgroup rw\n"))
	// Line containing a container ID in the expected position
	f.Add([]byte("600 500 8:1 /var/lib/containerd/io.containerd.grpc.v1.cri/sandboxes/0cfa82bf3ab29da271548d6a044e95c948c6fd2f7578fb41833a44ca23da425f/hostname /etc/hostname rw,relatime - ext4 /dev/sda1 rw\n"))
	// Empty file
	f.Add([]byte(""))
	f.Add([]byte("\n"))
	// Just whitespace
	f.Add([]byte("   \n"))
	// Partial line with no dash separator
	f.Add([]byte("557 530 0:148 / /sys/fs/cgroup rw\n"))
	// Line with sandboxes prefix (should be skipped)
	f.Add([]byte("600 500 8:1 /var/lib/containerd/io.containerd.grpc.v1.cri/sandboxes/abc123/hostname /etc/hostname rw - ext4 /dev/sda1 rw\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		f, err := os.CreateTemp("", "fuzz-mountinfo-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(data); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()

		_, _ = parseMountinfo(f.Name())
	})
}
