// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/common/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/sys/unix"
)

// Funcs mainly copied from github.com/DataDog/datadog-agent/pkg/security/utils/cgroup.go
// in order to reduce the binary size of cws-instrumentation

type controlGroup struct {
	// id unique hierarchy ID
	id int

	// controllers are the list of cgroup controllers bound to the hierarchy
	controllers []string

	// path is the pathname of the control group to which the process
	// belongs. It is relative to the mountpoint of the hierarchy.
	path string
}

func getProcControlGroupsFromFile(path string) ([]controlGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cgroups []controlGroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		t := scanner.Text()
		parts := strings.Split(t, ":")
		var ID int
		ID, err = strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		c := controlGroup{
			id:          ID,
			controllers: strings.Split(parts[1], ","),
			path:        parts[2],
		}
		cgroups = append(cgroups, c)
	}
	return cgroups, nil

}

func getCurrentProcContainerID() (string, error) {
	cgroups, err := getProcControlGroupsFromFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}

	for _, cgroup := range cgroups {
		cid := containerutils.FindContainerID(cgroup.path)
		if cid != "" {
			return cid, nil
		}
	}
	return "", nil
}

func getNSID() uint64 {
	var stat syscall.Stat_t
	if err := syscall.Stat("/proc/self/ns/pid", &stat); err != nil {
		return rand.Uint64()
	}
	return stat.Ino
}

// simpleHTTPRequest used to avoid importing the crypto golang package
func simpleHTTPRequest(uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	addr := u.Host
	if u.Port() == "" {
		addr += ":80"
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	client, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	path := u.Path
	if path == "" {
		path = "/"
	}

	req := fmt.Sprintf("GET %s?%s HTTP/1.1\nHost: %s\nConnection: close\n\n", path, u.RawQuery, u.Hostname())

	_, err = client.Write([]byte(req))
	if err != nil {
		return nil, err
	}

	var body []byte
	buf := make([]byte, 256)

	for {
		n, err := client.Read(buf)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		body = append(body, buf[:n]...)
	}

	offset := bytes.Index(body, []byte{'\r', '\n', '\r', '\n'})
	if offset < 0 {

		return nil, errors.New("unable to parse http response")
	}

	return body[offset+2:], nil
}

func fillProcessCwd(process *Process) error {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", process.Pid))
	if err != nil {
		return err
	}
	process.Res.Cwd = cwd
	return nil
}

func getFullPathFromFd(process *Process, filename string, fd int32) (string, error) {
	if len(filename) > 0 && filename[0] != '/' {
		if fd == unix.AT_FDCWD { // if use current dir, try to prefix it
			if process.Res.Cwd != "" || fillProcessCwd(process) == nil {
				filename = filepath.Join(process.Res.Cwd, filename)
			} else {
				return "", errors.New("fillProcessCwd failed")
			}
		} else { // if using another dir, prefix it, we should have it in cache
			if path, exists := process.Res.Fd[fd]; exists {
				filename = filepath.Join(path, filename)
			} else {
				return "", errors.New("process FD cache incomplete during path resolution")
			}
		}
	}
	return filename, nil
}

func getFullPathFromFilename(process *Process, filename string) (string, error) {
	if filename[0] != '/' {
		if process.Res.Cwd != "" || fillProcessCwd(process) == nil {
			filename = filepath.Join(process.Res.Cwd, filename)
		} else {
			return "", errors.New("fillProcessCwd failed")
		}
	}
	return filename, nil
}

func fillFileMetadata(filepath string, openMsg *ebpfless.OpenSyscallMsg, disableStats bool) error {
	if disableStats || strings.HasPrefix(filepath, "memfd:") {
		return nil
	}

	// NB: Here we use Lstat to not follow the link, because we don't do it yet globally.
	//     Once we'll follow them, we may want to replace it by a Stat().
	fileInfo, err := os.Lstat(filepath)
	if err != nil {
		return nil
	}
	stat := fileInfo.Sys().(*syscall.Stat_t)
	openMsg.MTime = uint64(stat.Mtim.Nano())
	openMsg.CTime = uint64(stat.Ctim.Nano())
	openMsg.Credentials = &ebpfless.Credentials{
		UID: stat.Uid,
		GID: stat.Gid,
	}
	if openMsg.Mode == 0 { // here, mode can be already set by handler of open syscalls
		openMsg.Mode = stat.Mode // useful for exec handlers
	}
	return nil
}

func getPidTTY(pid int) string {
	tty, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/0", pid))
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(tty, "/dev/pts") {
		return ""
	}
	return "pts" + path.Base(tty)
}

func truncateArgs(list []string) ([]string, bool) {
	truncated := false
	if len(list) > model.MaxArgsEnvsSize {
		list = list[:model.MaxArgsEnvsSize]
		truncated = true
	}
	for i, l := range list {
		if len(l) > model.MaxArgEnvSize {
			list[i] = l[:model.MaxArgEnvSize-4] + "..."
			truncated = true
		}
	}
	return list, truncated
}

// list copied from default value of env_with_value system-probe config
var priorityEnvs = []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "HISTSIZE", "HISTFILESIZE", "GLIBC_TUNABLES"}

func truncateEnvs(list []string) ([]string, bool) {
	truncated := false
	if len(list) > model.MaxArgsEnvsSize {
		// walk over all envs and put priority ones asides
		var priorityList []string
		var secondaryList []string
		for _, l := range list {
			found := false
			for _, prio := range priorityEnvs {
				if strings.HasPrefix(l, prio) {
					priorityList = append(priorityList, l)
					found = true
					break
				}
			}
			if !found {
				secondaryList = append(secondaryList, l)
			}
		}
		// build the result by first taking the priority envs if found
		list = append(priorityList, secondaryList[:model.MaxArgsEnvsSize-len(priorityList)]...)
		truncated = true
	}
	for i, l := range list {
		if len(l) > model.MaxArgEnvSize {
			list[i] = l[:model.MaxArgEnvSize-4] + "..."
			truncated = true
		}
	}
	return list, truncated
}

func secsToNanosecs(secs uint64) uint64 {
	return secs * 1000000000
}

func microsecsToNanosecs(secs uint64) uint64 {
	return secs * 1000
}
