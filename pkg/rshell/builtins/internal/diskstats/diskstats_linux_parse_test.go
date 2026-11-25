// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package diskstats

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const sampleMountInfo = `36 35 98:0 / / rw,noatime - ext4 /dev/sda1 rw,errors=remount-ro
37 36 0:18 / /sys rw,nosuid,nodev,noexec,relatime - sysfs sysfs rw
38 36 0:4 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
39 36 0:6 / /dev rw,nosuid - devtmpfs udev rw,size=4M
40 36 0:23 / /run rw,nosuid,nodev - tmpfs tmpfs rw,size=812M
41 36 0:25 / /mnt/with\040space rw - ext4 /dev/sdb1 rw
42 36 0:26 / /home/user rw - nfs server:/export rw
`

func TestParseMountInfo_HappyPath(t *testing.T) {
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(sampleMountInfo))
	assert.NoError(t, err)
	assert.Len(t, mounts, 7)

	assert.Equal(t, "/", mounts[0].MountPoint)
	assert.Equal(t, "ext4", mounts[0].FSType)
	assert.Equal(t, "/dev/sda1", mounts[0].Source)
	assert.Equal(t, "98:0", mounts[0].DevID, "DevID from mountinfo field 2")
	assert.False(t, mounts[0].Pseudo)
	assert.True(t, mounts[0].Local)

	// Pseudo filesystems are local in the GNU sense ("not remote")
	// so that `df -al` keeps them. -l only drops actually-remote
	// mounts (NFS / CIFS / fuse.sshfs).
	assert.Equal(t, "sysfs", mounts[1].FSType)
	assert.True(t, mounts[1].Pseudo)
	assert.True(t, mounts[1].Local)

	// devtmpfs reports real /dev contents and is intentionally NOT in
	// pseudoTypes (matches GNU df default listing).
	assert.Equal(t, "/dev", mounts[3].MountPoint)
	assert.Equal(t, "devtmpfs", mounts[3].FSType)
	assert.False(t, mounts[3].Pseudo)

	// tmpfs is /run in this fixture — also intentionally NOT pseudo.
	assert.Equal(t, "/run", mounts[4].MountPoint)
	assert.Equal(t, "tmpfs", mounts[4].FSType)
	assert.False(t, mounts[4].Pseudo)

	// Octal-escaped space.
	assert.Equal(t, "/mnt/with space", mounts[5].MountPoint)

	// NFS classified as remote.
	assert.Equal(t, "nfs", mounts[6].FSType)
	assert.False(t, mounts[6].Pseudo)
	assert.False(t, mounts[6].Local)
}

func TestParseMountInfo_SkipsMalformedLines(t *testing.T) {
	input := `not enough fields
36 35 98:0 / / rw - ext4 /dev/sda1 rw
short - bad
` + "37 36 0:18 / /sys rw - sysfs sysfs rw\n"
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(input))
	assert.NoError(t, err)
	assert.Len(t, mounts, 2, "should skip malformed lines silently")
}

func TestParseMountInfo_NoSeparator(t *testing.T) {
	mounts, err := parseMountInfo(context.Background(), strings.NewReader("36 35 98:0 / / rw ext4 /dev/sda1\n"))
	assert.NoError(t, err)
	assert.Empty(t, mounts, "lines without ' - ' must be skipped")
}

func TestParseMountInfo_LineTooLong(t *testing.T) {
	long := strings.Repeat("x", maxMountInfoLine+1) + "\n"
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(long))
	assert.ErrorIs(t, err, errLineTooLong)
	assert.Empty(t, mounts)
}

func TestParseMountInfo_TooManyMounts(t *testing.T) {
	var b strings.Builder
	for range MaxMounts + 5 {
		b.WriteString("36 35 98:0 / /m rw - ext4 /dev/x rw\n")
	}
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(b.String()))
	assert.ErrorIs(t, err, ErrMaxMounts)
	assert.Equal(t, MaxMounts, len(mounts))
}

func TestParseMountInfo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := parseMountInfo(ctx, strings.NewReader(sampleMountInfo))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestUnescapeMountField(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a\\040b", "a b"},
		{"a\\011b", "a\tb"},
		{"a\\012b", "a\nb"},
		{"a\\134b", "a\\b"},
		{"\\040leading", " leading"},
		{"trailing\\040", "trailing "},
		{"\\040", " "},
		// Invalid escape: not octal, kept literal.
		{"a\\999b", "a\\999b"},
		// Truncated escape at end: kept literal.
		{"a\\04", "a\\04"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, unescapeMountField(c.in), "in=%q", c.in)
	}
}

func TestParseMountInfoLine_FieldsAfterSeparator(t *testing.T) {
	// Optional fields between mount opts and the " - " separator.
	line := "36 35 98:0 / / rw shared:1 master:2 - ext4 /dev/sda1 rw,errors=remount-ro"
	m, ok := parseMountInfoLine(line)
	assert.True(t, ok)
	assert.Equal(t, "/", m.MountPoint)
	assert.Equal(t, "ext4", m.FSType)
}

func TestParseMountInfoLine_PostSeparatorTooFew(t *testing.T) {
	// Missing source field after fstype.
	_, ok := parseMountInfoLine("36 35 98:0 / / rw - ext4")
	assert.False(t, ok)
}

func TestIsRemoteSource_TypeOnly(t *testing.T) {
	// Source omitted (empty) — only the type prefix list applies.
	for _, fs := range []string{
		"nfs", "nfs4", "cifs", "smb3", "smbfs", "afs", "ceph",
		"glusterfs", "sshfs", "davfs",
		// FUSE subtypes: must match the explicit "fuse.<backend>"
		// form, not the short backend name. This is critical for
		// `df -l` hang protection on stale sshfs / smbnetfs mounts.
		"fuse.sshfs", "fuse.smbnetfs", "fuse.cifs", "fuse.davfs2",
		"fuse.glusterfs", "fuse.cephfs", "fuse.nfsv4", "fuse.s3fs",
		"fuse.rclone",
	} {
		assert.True(t, isRemoteSource("/dev/local", fs), fs)
	}
	for _, fs := range []string{
		"ext4", "btrfs", "xfs", "tmpfs", "apfs",
		// FUSE local backends must NOT be classified remote.
		"fuse.gvfsd-fuse", "fuse.portal", "fuse.archivemount",
	} {
		assert.False(t, isRemoteSource("/dev/local", fs), fs)
	}
}

// Mounts whose type is generic (gpfs, acfs, "auto", custom NFS-flavored
// types) but whose source carries a network signature must still be
// classified remote, matching GNU df's classification. Without this,
// `df -l` would keep the mount and statfs(2) could hang on a stale
// network mount.
func TestIsRemoteSource_BySourceShape(t *testing.T) {
	for _, src := range []string{
		"server:/export",        // classic NFS
		"user@host:/path",       // sshfs / autofs SSHFS
		"//server/share",        // SMB / CIFS UNC
		"nfs.example.com:/data", // NFS with FQDN
	} {
		assert.True(t, isRemoteSource(src, "ext4"), src)
		assert.True(t, isRemoteSource(src, "auto"), src)
		assert.True(t, isRemoteSource(src, "gpfs"), src)
	}
	// Local sources must remain local even with otherwise-suspicious
	// types (no colon, no leading "//").
	for _, src := range []string{
		"/dev/sda1",
		"/dev/disk/by-uuid/abcd",
		"tmpfs",
	} {
		assert.False(t, isRemoteSource(src, "ext4"), src)
		assert.False(t, isRemoteSource(src, "tmpfs"), src)
	}
}

func TestIsOctal(t *testing.T) {
	for _, b := range []byte("01234567") {
		assert.True(t, isOctal(b))
	}
	for _, b := range []byte("89abAB.\\") {
		assert.False(t, isOctal(b))
	}
}

func TestList_LiveHost_Linux(t *testing.T) {
	mounts, err := List(context.Background(), nil)
	if err != nil && errors.Is(err, ErrNotSupported) {
		t.Skipf("not supported on this platform: %v", err)
	}
	assert.NoError(t, err)
	// On a typical Linux host, "/" is mounted; on stripped-down
	// environments (some CI runners with mountinfo locked down) the
	// listing may be empty — accept that too.
	if len(mounts) > 0 {
		var foundRoot bool
		for _, m := range mounts {
			if m.MountPoint == "/" {
				foundRoot = true
				break
			}
		}
		// Permit no-root environments (some sandboxes) but if root
		// is present, validate that its block fields are populated.
		// NotZero on Total / BlockSize would catch silent statfs
		// failures or regressions in the Frsize-vs-Bsize fallback.
		if foundRoot {
			for _, m := range mounts {
				if m.MountPoint == "/" {
					assert.NotEmpty(t, m.FSType)
					assert.NotZero(t, m.Total, "root must report non-zero total")
					assert.NotZero(t, m.BlockSize, "root must report non-zero block size")
					break
				}
			}
		}
	}
}
