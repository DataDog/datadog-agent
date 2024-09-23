// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"bytes"
	"fmt"
	"text/tabwriter"
)

/*
Abstract of lsof output (see `man lsof` for details):

COMMAND  PID   USER   FD      TYPE             DEVICE SIZE/OFF       NODE NAME
systemd 1477 ubuntu  rtd       DIR              259,1     4096          2 /
systemd 1477 ubuntu  txt       REG              259,1  1849992       3324 /usr/lib/systemd/systemd
systemd 1477 ubuntu  mem       REG              259,1   613064       4798 /usr/lib/x86_64-linux-gnu/libpcre2-8.so.0.10.4
systemd 1477 ubuntu    0r      CHR                1,3      0t0          5 /dev/null
systemd 1477 ubuntu    1u     unix 0x0000000000000000      0t0      21781 type=STREAM
systemd 1477 ubuntu    3u     unix 0x0000000000000000      0t0      21261 type=DGRAM
systemd 1477 ubuntu    4u  a_inode               0,15        0         51 [eventpoll]
systemd 1477 ubuntu    5u  a_inode               0,15        0         51 [signalfd]
systemd 1477 ubuntu    6r  a_inode               0,15        0         51 inotify
systemd 1477 ubuntu    7r      DIR               0,28        0       4945 /sys/fs/cgroup/user.slice/user-1000.slice/user@1000.service
systemd 1477 ubuntu    8u  a_inode               0,15        0         51 [timerfd]
systemd 1477 ubuntu    9u  a_inode               0,15        0         51 [eventpoll]
systemd 1477 ubuntu   14r      REG               0,22        0 4026532073 /proc/swaps
systemd 1477 ubuntu   15u  netlink                         0t0      21277 KOBJECT_UEVENT

Here we don't care about COMMAND, PID, and USER, since we only look at a single process
*/

// File represents an open file
// The fields are not guaranteed to match lsof output, but they are good enough for debugging
type File struct {
	Fd       string
	Type     string
	OpenPerm string
	FilePerm string
	Size     int64
	Name     string
}

// Files represents a list of open files
type Files []File

func (files Files) String() string {
	var out bytes.Buffer
	writer := tabwriter.NewWriter(&out, 1, 1, 1, ' ', 0)

	fmt.Fprint(writer, "FD\tType\tSize\tOpenPerm\tFilePerm\tName\t\n")
	for _, file := range files {
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\t%s\t\n", file.Fd, file.Type, file.Size, file.OpenPerm, file.FilePerm, file.Name)
	}

	_ = writer.Flush()
	return out.String()
}
