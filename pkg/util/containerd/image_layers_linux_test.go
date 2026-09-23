// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/stretchr/testify/require"
)

func TestValidateHiddenBytesMounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mounts []mount.Mount
		count  int
		valid  bool
	}{
		{"native view", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top:/base", "index=off"}}}, 2, true},
		{"userxattr", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top:/base", "userxattr"}}}, 2, true},
		{"single bind", []mount.Mount{{Type: "bind", Source: "/base", Options: []string{"ro", "rbind"}}}, 1, true},
		{"writable bind", []mount.Mount{{Type: "bind", Source: "/base", Options: []string{"rw", "rbind"}}}, 1, false},
		{"merged bind", []mount.Mount{{Type: "bind", Source: "/merged", Options: []string{"ro", "rbind"}}}, 2, false},
		{"active upper", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/base", "upperdir=/running"}}}, 1, false},
		{"idmap", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top:/base", "uidmap=0:1:2"}}}, 2, false},
		{"metacopy", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top:/base", "metacopy=on"}}}, 2, false},
		{"remote mount", []mount.Mount{{Type: "fuse", Source: "/base", Options: []string{"ro"}}}, 1, false},
		{"empty path", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top::/base"}}}, 3, false},
		{"relative", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=top:base"}}}, 2, false},
		{"escaped", []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top\\:other:/base"}}}, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHiddenBytesMounts(tc.mounts, tc.count)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
