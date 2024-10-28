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
	"debug/elf"
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

	"golang.org/x/sys/unix"

	usergrouputils "github.com/DataDog/datadog-agent/pkg/security/common/usergrouputils"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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

func getContainerIDFromProcFS(cgroupPath string) (string, error) {
	cgroups, err := getProcControlGroupsFromFile(cgroupPath)
	if err != nil {
		return "", err
	}

	for _, cgroup := range cgroups {
		if cid, _ := containerutils.FindContainerID(cgroup.path); cid != "" {
			return cid, nil
		}
	}
	return "", nil
}

func getCurrentProcContainerID() (string, error) {
	return getContainerIDFromProcFS("/proc/self/cgroup")
}

func getProcContainerID(pid int) (string, error) {
	return getContainerIDFromProcFS(fmt.Sprintf("/proc/%d/cgroup", pid))
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

	req := fmt.Sprintf("GET %s?%s HTTP/1.0\nHost: %s\nConnection: close\n\n", path, u.RawQuery, u.Hostname())

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
	process.FsRes.Cwd = cwd
	return nil
}

func getFullPathFromFd(process *Process, filename string, fd int32) (string, error) {
	if len(filename) > 0 && filename[0] != '/' {
		if fd == unix.AT_FDCWD { // if use current dir, try to prefix it
			if process.FsRes.Cwd != "" || fillProcessCwd(process) == nil {
				filename = filepath.Join(process.FsRes.Cwd, filename)
			} else {
				return "", errors.New("fillProcessCwd failed")
			}
		} else { // if using another dir, prefix it, we should have it in cache
			path, err := process.GetFilenameFromFd(fd)
			if err != nil {
				return "", fmt.Errorf("process FD cache incomplete during path resolution: %w", err)
			}

			filename = filepath.Join(path, filename)
		}
	}
	return filename, nil
}

func getFullPathFromFilename(process *Process, filename string) (string, error) {
	if len(filename) > 0 && filename[0] != '/' {
		if process.FsRes.Cwd != "" || fillProcessCwd(process) == nil {
			filename = filepath.Join(process.FsRes.Cwd, filename)
		} else {
			return "", errors.New("fillProcessCwd failed")
		}
	}
	return filename, nil
}

func refreshUserCache(tracer *Tracer) error {
	file, err := os.Open(passwdPath)
	if err != nil {
		return err
	}
	defer file.Close()
	cache, err := usergrouputils.ParsePasswdFile(file)
	if err != nil {
		return err
	}
	tracer.userCache = cache
	return nil
}

func refreshGroupCache(tracer *Tracer) error {
	file, err := os.Open(groupPath)
	if err != nil {
		return err
	}
	defer file.Close()
	cache, err := usergrouputils.ParseGroupFile(file)
	if err != nil {
		return err
	}
	tracer.groupCache = cache
	return nil
}

func getFileMTime(filepath string) uint64 {
	fileInfo, err := os.Lstat(filepath)
	if err != nil {
		return 0
	}
	stat := fileInfo.Sys().(*syscall.Stat_t)
	return uint64(stat.Mtim.Nano())
}

func getUserFromUID(tracer *Tracer, uid int32) string {
	if uid < 0 {
		return ""
	}
	// if it's the first time, or if passwd was updated, load/refresh the user cache
	if tracer.userCache == nil {
		tracer.lastPasswdMTime = getFileMTime(passwdPath)
		if err := refreshUserCache(tracer); err != nil {
			return ""
		}
	} else if tracer.userCacheRefreshLimiter.Allow() {
		// refresh the cache only if the file has changed
		mtime := getFileMTime(passwdPath)
		if mtime != tracer.lastPasswdMTime {
			if err := refreshUserCache(tracer); err != nil {
				return ""
			}
			tracer.lastPasswdMTime = mtime
		}
	}
	if user, found := tracer.userCache[int(uid)]; found {
		return user
	}
	return ""
}

func getGroupFromGID(tracer *Tracer, gid int32) string {
	if gid < 0 {
		return ""
	}
	// if it's the first time, or if group was updated, load/refresh the group cache
	if tracer.groupCache == nil {
		tracer.lastGroupMTime = getFileMTime("/etc/group")
		if err := refreshGroupCache(tracer); err != nil {
			return ""
		}
	} else if tracer.groupCacheRefreshLimiter.Allow() {
		// refresh the cache only if the file has changed
		mtime := getFileMTime("/etc/group")
		if mtime != tracer.lastGroupMTime {
			if err := refreshGroupCache(tracer); err != nil {
				return ""
			}
			tracer.lastGroupMTime = mtime
		}
	}
	if group, found := tracer.groupCache[int(gid)]; found {
		return group
	}
	return ""
}

func fillFileMetadata(tracer *Tracer, filepath string, fileMsg *ebpfless.FileSyscallMsg, disableStats bool) error {
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
	fileMsg.MTime = uint64(stat.Mtim.Nano())
	fileMsg.CTime = uint64(stat.Ctim.Nano())
	fileMsg.Inode = stat.Ino
	fileMsg.Credentials = &ebpfless.Credentials{
		UID:   stat.Uid,
		User:  getUserFromUID(tracer, int32(stat.Uid)),
		GID:   stat.Gid,
		Group: getGroupFromGID(tracer, int32(stat.Gid)),
	}
	if fileMsg.Mode == 0 { // here, mode can be already set by handler of open syscalls
		fileMsg.Mode = stat.Mode // useful for exec handlers
	}
	return nil
}

func getPidTTY(pid int) string {
	ttyPath, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/0", pid))
	if err != nil {
		return ""
	}
	if ttyPath == "/dev/null" {
		return ""
	}

	if strings.HasPrefix(ttyPath, "/dev/pts") {
		return "pts" + path.Base(ttyPath)
	}

	if strings.HasPrefix(ttyPath, "/dev") {
		return path.Base(ttyPath)
	}

	return ""
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
var priorityEnvsPrefixes = []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "HISTSIZE", "HISTFILESIZE", "GLIBC_TUNABLES"}

// StringIterator defines a string iterator
type StringIterator interface {
	Next() bool
	Text() string
	Reset()
}

// StringArrayIterator defines a string array iterator
type StringArrayIterator struct {
	array []string
	curr  int
	next  int
}

// NewStringArrayIterator returns a new string array iterator
func NewStringArrayIterator(array []string) *StringArrayIterator {
	return &StringArrayIterator{
		array: array,
	}
}

// Next returns true if there is a next element
func (s *StringArrayIterator) Next() bool {
	if s.next >= len(s.array) {
		return false
	}
	s.curr = s.next
	s.next++

	return true
}

// Text return the current element
func (s *StringArrayIterator) Text() string {
	return s.array[s.curr]
}

// Reset reset the iterator
func (s *StringArrayIterator) Reset() {
	s.curr, s.next = 0, 0
}

func truncateEnvs(it StringIterator) ([]string, bool) {
	var (
		priorityEnvs []string
		envCounter   int
		truncated    bool
	)

	for it.Next() {
		text := it.Text()
		if len(text) > 0 {
			envCounter++
			if matchesOnePrefix(text, priorityEnvsPrefixes) {
				if len(text) > model.MaxArgEnvSize {
					text = text[:model.MaxArgEnvSize-4] + "..."
					truncated = true
				}
				priorityEnvs = append(priorityEnvs, text)
			}
		}
	}

	it.Reset()

	if envCounter > model.MaxArgsEnvsSize {
		envCounter = model.MaxArgsEnvsSize
	}

	// second pass collecting
	envs := make([]string, 0, envCounter)
	envs = append(envs, priorityEnvs...)

	for it.Next() {
		if len(envs) >= model.MaxArgsEnvsSize {
			return envs, true
		}

		text := it.Text()
		if len(text) > 0 {
			// if it matches one prefix, it's already in the envs through priority envs
			if !matchesOnePrefix(text, priorityEnvsPrefixes) {
				if len(text) > model.MaxArgEnvSize {
					text = text[:model.MaxArgEnvSize-4] + "..."
					truncated = true
				}
				envs = append(envs, text)
			}
		}
	}

	return envs, truncated
}

func secsToNanosecs(secs uint64) uint64 {
	return secs * 1000000000
}

func microsecsToNanosecs(secs uint64) uint64 {
	return secs * 1000
}

func getModuleName(reader io.ReaderAt) (string, error) {
	elf, err := elf.NewFile(reader)
	if err != nil {
		return "", err
	}
	defer elf.Close()
	section := elf.Section(".gnu.linkonce.this_module")
	if section == nil {
		return "", errors.New("found no '.gnu.linkonce.this_module' section")
	}
	data, err := section.Data()
	if err != nil {
		return "", err
	} else if len(data) < 25 {
		return "", errors.New("section data too short")
	}
	index := bytes.IndexByte(data[24:], 0) // 24 is the offset on 64bits
	if index == -1 {
		return "", errors.New("no string found")
	}
	return string(data[24 : 24+index]), nil
}

func getModuleNameFromFile(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return getModuleName(file)
}
