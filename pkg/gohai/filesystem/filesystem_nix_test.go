// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin
// +build linux darwin

package filesystem

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func withDfCommand(t *testing.T, command ...string) {
	oldCommand := dfCommand
	oldOptions := dfOptions
	dfCommand = command[0]
	if len(command) > 1 {
		dfOptions = command[1:]
	} else {
		dfOptions = []string{}
	}
	t.Cleanup(func() {
		dfCommand = oldCommand
		dfOptions = oldOptions
	})
}

func TestSlowDf(t *testing.T) {
	withDfCommand(t, "sleep", "5")
	dfTimeout = 20 * time.Millisecond // test faster
	defer func() { dfTimeout = 2 * time.Second }()

	_, err := getFileSystemInfo()
	require.ErrorContains(t, err, "df failed to collect filesystem data")
}

func TestOldMacosDf(t *testing.T) {
	// from https://apple.stackexchange.com/questions/263437/df-hide-ifree-iused-512-blocks-customize-column-format-dont-show-inode-info
	withDfCommand(t, "sh", "-c", `
		echo 'Filesystem                                 1K-blocks       Used Available Capacity  iused    ifree %iused  Mounted on';
		echo '/dev/disk0s2                               975093952  719904648 254677304    74% 90052079 31834663   74%   /';
		echo 'devfs                                            368        368         0   100%      637        0  100%   /dev';
		echo 'map -hosts                                         0          0         0   100%        0        0  100%   /net';
		echo 'map -static                                        0          0         0   100%        0        0  100%   /Volumes/Large';
	`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]string{"kb_size": "975093952", "mounted_on": "/", "name": "/dev/disk0s2"},
		map[string]string{"kb_size": "368", "mounted_on": "/dev", "name": "devfs"},
		map[string]string{"kb_size": "0", "mounted_on": "/net", "name": "map -hosts"},
		map[string]string{"kb_size": "0", "mounted_on": "/Volumes/Large", "name": "map -static"},
	}, out)
}

func TestDfLinux(t *testing.T) {
	withDfCommand(t, "sh", "-c", `
		echo 'Filesystem             1K-blocks     Used Available Use% Mounted on';
		echo '/dev/root               16197480 13252004   2929092  82% /';
		echo 'devtmpfs                15381564        0  15381564   0% /dev';
		echo 'tmpfs                   15388388        0  15388388   0% /dev/shm';
	`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]string{"kb_size": "16197480", "mounted_on": "/", "name": "/dev/root"},
		map[string]string{"kb_size": "15381564", "mounted_on": "/dev", "name": "devtmpfs"},
		map[string]string{"kb_size": "15388388", "mounted_on": "/dev/shm", "name": "tmpfs"},
	}, out)
}

func TestDfMac(t *testing.T) {
	withDfCommand(t, "sh", "-c", `
		echo 'Filesystem     1024-blocks      Used Available Capacity iused      ifree %iused  Mounted on';
		echo '/dev/disk1s1s1   488245288  15055192 344743840     5%  502048 3447438400    0%   /';
		echo '/dev/disk1s5     488245288        20 344743840     1%       2 3447438400    0%   /System/Volumes/VM';
	`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]string{"kb_size": "488245288", "mounted_on": "/", "name": "/dev/disk1s1s1"},
		map[string]string{"kb_size": "488245288", "mounted_on": "/System/Volumes/VM", "name": "/dev/disk1s5"},
	}, out)
}

func TestDfWithVolumeSpaces(t *testing.T) {
	withDfCommand(t, "sh", "-c", `
		echo 'Filesystem     1K-blocks      Used Available Use% Mounted on';
		echo '/dev/disk4s3      367616    360928      6688  99% /Volumes/Firefox';
		echo '/dev/disk5        307200    283136     24064  93% /Volumes/MySQL Workbench community-8.0.30';
	`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]string{"kb_size": "367616", "mounted_on": "/Volumes/Firefox", "name": "/dev/disk4s3"},
		map[string]string{"kb_size": "307200", "mounted_on": "/Volumes/MySQL Workbench community-8.0.30", "name": "/dev/disk5"},
	}, out)
}

func TestDfWithErrors(t *testing.T) {
	withDfCommand(t, "sh", "-c", `
		echo 'Filesystem     1K-blocks      Used Available Use% Mounted on';
		echo '/dev/disk4s3      367616    360928      6688  99% /Volumes/Firefox';
		echo 'Some error from df';
		echo '/dev/disk5        307200    283136     24064  93% /Volumes/MySQL Workbench community-8.0.30';
	`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]string{"kb_size": "367616", "mounted_on": "/Volumes/Firefox", "name": "/dev/disk4s3"},
		map[string]string{"kb_size": "307200", "mounted_on": "/Volumes/MySQL Workbench community-8.0.30", "name": "/dev/disk5"},
	}, out)
}

func TestFaileDfWithData(t *testing.T) {
	// (note that this sample output is valid on both linux and darwin)
	withDfCommand(t, "sh", "-c", `echo "Filesystem     1K-blocks      Used Available Use% Mounted on"; echo "/dev/disk1s1s1 488245288 138504332 349740956  29% /"; exit 1`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]string{"kb_size": "488245288", "mounted_on": "/", "name": "/dev/disk1s1s1"},
	}, out)
}

func TestGetFileSystemInfo(t *testing.T) {
	out, err := getFileSystemInfo()
	require.NoError(t, err)
	outArray := out.([]interface{})
	require.Greater(t, len(outArray), 0)
}
