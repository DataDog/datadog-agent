// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build darwin || linux
// +build darwin linux

package filesystem

import (
	"math/rand"
	"testing"

	"github.com/moby/sys/mountinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Used for dynamic test field value generation
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func TestNixFSTypeFiltering(t *testing.T) {
	mockFSFunctions(t)

	var testCases = []struct {
		FSType   string
		FSName   string
		Included bool
	}{
		// Sample of some FS types that are not ignored

		{"ext3", randString(), true},
		{"ext4", randString(), true},
		{"apfs", randString(), true},
		{"aufs", randString(), true},

		// Basic ignored FS types

		{"autofs", randString(), false},
		{"debugfs", randString(), false},
		{"devfs", randString(), false},
		{"devpts", randString(), false},
		{"devtmpfs", randString(), false},
		{"fuse.portal", randString(), false},
		{"fusectl", randString(), false},
		{"ignore", randString(), false},
		{"kernfs", randString(), false},
		{"none", randString(), false},
		{"proc", randString(), false},
		{"subfs", randString(), false},
		{"mqueue", randString(), false},
		{"rpc_pipefs", randString(), false},
		{"squashfs", randString(), false},
		{"sysfs", randString(), false},

		// Remote/Networked FS types detected by FSType

		{"acfs", randString(), false},
		{"afs", randString(), false},
		{"auristorfs", randString(), false},
		{"coda", randString(), false},
		{"fhgfs", randString(), false},
		{"gpfs", randString(), false},
		{"ibrix", randString(), false},
		{"ocfs2", randString(), false},
		{"vxfs", randString(), false},

		// Remote/Networked FS types detected by names

		// `-hosts` FSName is remote
		{"dummyhosts1", randString(), true},
		{"dummyhosts2", randString() + "-" + randString(), true},
		{"dummyhosts3", "-hosts", false},

		// Anything w/ `:`s is assumed to be a remote mount
		{"dummycolons1", randString(), true},
		{"dummycolons2", randString() + ":" + randString(), false},
		{"dummycolons3", ":" + randString(), false},

		// Anything starting with `//` and from a specific set of FS types (CIFS/SMB) is remote too
		{"dummyfwdslashes1", randString(), true},
		{"dummyfwdslashes2", "//" + randString(), true},
		{"dummyfwdslashes3", "/" + randString(), true},
		{"cifs", "//" + randString(), false},
		{"smb3", "//" + randString(), false},
		{"smbfs", "//" + randString(), false},
	}

	caser := cases.Title(language.English)
	for _, tc := range testCases {
		tc := tc
		t.Run("TestIgnoringOfFSType/"+caser.String(tc.FSType), func(t *testing.T) {
			inputMounts := []*mountinfo.Info{
				{
					Source:     tc.FSName,
					FSType:     tc.FSType,
					Mountpoint: randString(),
				},
				newTestInputMountinfo(randString()),
				newTestInputMountinfo(randString()),
				{
					Source:     tc.FSName,
					FSType:     tc.FSType,
					Mountpoint: randString(),
				},
				newTestInputMountinfo(randString()),
			}

			expectedMounts := make([]MountInfo, 0, len(inputMounts))
			for _, mount := range inputMounts {
				// We only care about excluding specific FS types that we are doing the
				// test for and that are marked as excluded
				if mount.FSType == tc.FSType {
					if !tc.Included {
						continue
					}
				}

				expectedMount := MountInfo{
					Name:      mount.Source,
					MountedOn: mount.Mountpoint,
					SizeKB:    1,
				}

				expectedMounts = append(expectedMounts, expectedMount)
			}

			mounts, err := getFileSystemInfoWithMounts(inputMounts)
			require.NoError(t, err)

			require.Equal(t, len(expectedMounts), len(mounts))
			assert.ElementsMatch(t, mounts, expectedMounts)
		})
	}
}

func TestNixMissingMountValues(t *testing.T) {
	mockFSFunctions(t)

	var testCases = []struct {
		Desc           string
		InputMounts    []*mountinfo.Info
		ExpectedMounts []MountInfo
	}{
		{
			"MissingSize",
			[]*mountinfo.Info{newTestInputMountinfo("Normal1")},
			[]MountInfo{newTestOutputMountInfo("Normal1")},
		},
		{
			"MissingMountName",
			[]*mountinfo.Info{
				newTestInputMountinfo("Normal1"),
				{Source: "", FSType: "foo", Mountpoint: "Bad1"},
				newTestInputMountinfo("Normal2"),
				{Source: "none", FSType: "foo", Mountpoint: "Bad2"},
				newTestInputMountinfo("Normal3"),
			},
			[]MountInfo{
				newTestOutputMountInfo("Normal1"),
				newTestOutputMountInfo("Normal2"),
				newTestOutputMountInfo("Normal3"),
			},
		},
		{
			"MissingMountPoint",
			[]*mountinfo.Info{
				newTestInputMountinfo("Normal1"),
				{Source: "Bad1", FSType: "foo", Mountpoint: ""},
				newTestInputMountinfo("Normal2"),
				{Source: "Bad2", FSType: "foo", Mountpoint: ""},
				newTestInputMountinfo("Normal3"),
			},
			[]MountInfo{
				newTestOutputMountInfo("Normal1"),
				newTestOutputMountInfo("Normal2"),
				newTestOutputMountInfo("Normal3"),
			},
		},
		{
			"MissingMountPointAndName",
			[]*mountinfo.Info{
				newTestInputMountinfo("Normal1"),
				{Source: "", FSType: "foo", Mountpoint: ""},
				newTestInputMountinfo("Normal2"),
			},
			[]MountInfo{
				newTestOutputMountInfo("Normal1"),
				newTestOutputMountInfo("Normal2"),
			},
		},
		{
			"MissingFSType",
			[]*mountinfo.Info{
				newTestInputMountinfo("Normal1"),
				{Source: "Bad1", FSType: "", Mountpoint: "Bad1"},
				newTestInputMountinfo("Normal2"),
				{Source: "Bad2", FSType: "", Mountpoint: "Bad2"},
				newTestInputMountinfo("Normal3"),
			},
			[]MountInfo{
				newTestOutputMountInfo("Normal1"),
				newTestOutputMountInfo("Normal2"),
				newTestOutputMountInfo("Normal3"),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Desc, func(t *testing.T) {
			mounts, err := getFileSystemInfoWithMounts(tc.InputMounts)
			require.NoError(t, err)

			require.Equal(t, len(tc.ExpectedMounts), len(mounts))
			assert.ElementsMatch(t, mounts, tc.ExpectedMounts)
		})
	}
}

func TestFilterEmptySize(t *testing.T) {
	mockFSFunctions(t)
	getFSSizeFn = func(mount *mountinfo.Info) (uint64, error) {
		return 0, nil
	}

	initialMounts := []*mountinfo.Info{
		newTestInputMountinfo("Normal"),
	}
	mounts, err := getFileSystemInfoWithMounts(initialMounts)
	require.NoError(t, err)
	require.Empty(t, mounts)
}

func TestFilterDev(t *testing.T) {
	mockFSFunctions(t)
	getFSDevFn = func(mount *mountinfo.Info) (interface{}, error) {
		return 1, nil
	}

	var testCases = []struct {
		Desc           string
		InputMounts    []*mountinfo.Info
		ExpectedMounts []MountInfo
	}{
		{
			"ReplaceDevNameSlash",
			[]*mountinfo.Info{
				{Source: "Source", FSType: "foo", Mountpoint: "MountPoint"},
				{Source: "/Source", FSType: "foo", Mountpoint: "MountPoint"},
			},
			[]MountInfo{
				{Name: "/Source", SizeKB: 1, MountedOn: "MountPoint"},
			},
		},
		{
			"ReplaceDevMountLength",
			[]*mountinfo.Info{
				{Source: "Source", FSType: "foo", Mountpoint: "MountPoint0"},
				{Source: "Source", FSType: "foo", Mountpoint: "MountPoint"},
			},
			[]MountInfo{
				{Name: "Source", SizeKB: 1, MountedOn: "MountPoint"},
			},
		},
		{
			"ReplaceDevNewSource",
			[]*mountinfo.Info{
				{Source: "Source", FSType: "foo", Mountpoint: "MountPoint"},
				{Source: "NewSource", FSType: "foo", Mountpoint: "MountPoint"},
			},
			[]MountInfo{
				{Name: "NewSource", SizeKB: 1, MountedOn: "MountPoint"},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Desc, func(t *testing.T) {
			mounts, err := getFileSystemInfoWithMounts(tc.InputMounts)
			require.NoError(t, err)

			require.Equal(t, len(tc.ExpectedMounts), len(mounts))
			assert.ElementsMatch(t, mounts, tc.ExpectedMounts)
		})
	}
}

// Test Helpers

func newTestInputMountinfo(name string) *mountinfo.Info {
	return &mountinfo.Info{
		Source:     name + "Source",
		FSType:     name,
		Mountpoint: name + "MountPoint",
	}
}

func newTestOutputMountInfo(name string) MountInfo {
	return MountInfo{
		Name:      name + "Source",
		MountedOn: name + "MountPoint",
		SizeKB:    1,
	}
}

func randString() string {
	stringLength := rand.Intn(30) + 1
	bytes := make([]byte, stringLength)
	for idx := range bytes {
		bytes[idx] = charset[rand.Intn(len(charset))]
	}
	return string(bytes)
}

func mockGetSize(t *testing.T) {
	getFSSizeFn = func(mount *mountinfo.Info) (uint64, error) {
		return 1, nil
	}

	t.Cleanup(func() {
		getFSSizeFn = getFSSize
	})
}

func mockGetDev(t *testing.T) {
	counter := 0
	getFSDevFn = func(mount *mountinfo.Info) (interface{}, error) {
		counter++
		return counter, nil
	}

	t.Cleanup(func() {
		getFSDevFn = getFSDev
	})
}

func mockFSFunctions(t *testing.T) {
	mockGetSize(t)
	mockGetDev(t)
}
