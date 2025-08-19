// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// see the documentation for /proc for more details about files and their format
// https://www.kernel.org/doc/html/latest/filesystems/proc.html

// openFilesLister stores the state needed to list open files
// this is helpful for tests to help with mocking
type openFilesLister struct {
	pid      int
	procPath string

	readlink func(string) (string, error)
	stat     func(string) (os.FileInfo, error)
	lstat    func(string) (os.FileInfo, error)

	proc       procfsProc
	socketInfo map[uint64]socketInfo
}

// procfsProc is an interface to allow mocking of procfs.Proc
type procfsProc interface {
	ProcMaps() ([]*procfs.ProcMap, error)
	FileDescriptors() ([]uintptr, error)
	Cwd() (string, error)
}

type socketInfo struct {
	Description string
	State       string
	Protocol    string
}

func openFiles(pid int) (Files, error) {
	ofl := &openFilesLister{
		pid: pid,

		readlink: os.Readlink,
		stat:     os.Stat,
		lstat:    os.Lstat,
	}

	ofl.procPath = procPath()

	fs, err := procfs.NewFS(ofl.procPath)
	if err != nil {
		return nil, err
	}

	ofl.proc, err = fs.Proc(pid)
	if err != nil {
		return nil, err
	}

	ofl.socketInfo = readSocketInfo(ofl.procPIDPath())

	return ofl.openFiles(), nil
}

func (ofl *openFilesLister) procPIDPath() string {
	return fmt.Sprintf("%s/%d", ofl.procPath, ofl.pid)
}

func (ofl *openFilesLister) openFiles() Files {
	var files Files

	// open files, socket, pipe (everything with a file descriptor, from /proc/<pid>/fd)
	openFDFiles, err := ofl.fdMetadata()
	if err != nil {
		log.Debugf("Failed to get open FDs for pid %d: %s", ofl.pid, err)
	} else {
		files = append(files, openFDFiles...)
	}

	// memory mapped files, code, regions (from /proc/<pid>/maps)
	mmapFiles, err := ofl.mmapMetadata()
	if err != nil {
		log.Debugf("Failed to get memory maps for pid %d: %s", ofl.pid, err)
	} else {
		files = append(files, mmapFiles...)
	}

	return files
}

func (ofl *openFilesLister) mmapMetadata() (Files, error) {
	maps, err := ofl.proc.ProcMaps()
	if err != nil {
		return nil, err
	}

	cwd, _ := ofl.proc.Cwd()

	var files Files
	for i, m := range maps {
		if i > 0 && m.Pathname == maps[i-1].Pathname {
			// skip duplicate entries
			continue
		}
		if m.Pathname == "" {
			// anonymous mapping
			continue
		}

		if m.Dev == 0 || m.Inode == 0 {
			// virtual memory region, eg. [heap], [stack], [vvar], etc
			continue
		}

		file := File{
			Name:     m.Pathname,
			OpenPerm: permToString(m.Perms),
		}

		if file.Type, file.FilePerm, file.Size, _ = fileStats(ofl.stat, m.Pathname); file.Type == "" {
			continue
		}
		file.Fd = mmapFD(m.Pathname, file.Type, cwd)

		files = append(files, file)
	}

	return files, nil
}

func permToString(perms *procfs.ProcMapPermissions) string {
	s := ""

	for _, perm := range []struct {
		set       bool
		charSet   string
		charUnset string
	}{
		{perms.Read, "r", "-"},
		{perms.Write, "w", "-"},
		{perms.Execute, "x", "-"},
		{perms.Private, "p", ""},
		{perms.Shared, "s", ""},
	} {
		if perm.set {
			s += perm.charSet
		} else {
			s += perm.charUnset
		}
	}

	return s
}

func mmapFD(path string, fileType, cwd string) string {
	/*
		cwd  current working directory;
		ltx  shared library text (code and data);
		mem  memory-mapped file;
		mmap memory-mapped device;
		rtd  root directory;
		txt  program text (code and data);
	*/

	if fileType == "REG" {
		// knowing whether the file is memory mapped or program text would require reading /proc/<pid>/stat
		// but the specific fields needed are not parsed by procfs.Proc
		// just assume it's a memory mapped file
		return "mem"
	}

	if fileType == "DIR" {
		if path == "/" {
			return "rtd"
		}
		if cwd != "" && path == cwd {
			return "cwd"
		}
	}
	return "unknown"
}

func (ofl *openFilesLister) fdMetadata() (Files, error) {
	openFDs, err := ofl.proc.FileDescriptors()
	if err != nil {
		return nil, err
	}

	var files Files
	for _, openFD := range openFDs {
		file, ok := ofl.fdStat(openFD)
		if ok {
			files = append(files, file)
		}
	}

	return files, nil
}

func (ofl *openFilesLister) fdStat(fd uintptr) (File, bool) {
	var err error
	file := File{
		Fd: fmt.Sprintf("%d", fd),
	}

	fdLinkPath := fmt.Sprintf("%s/fd/%d", ofl.procPIDPath(), fd)

	if file.Type, file.OpenPerm, _, _ = fileStats(ofl.lstat, fdLinkPath); file.Type == "" {
		return File{}, false
	}

	var ok bool
	// remove some unnecessary information from permissions string
	// if the expectations are not met, the permissions are left as is

	// file descriptors always have no sticky bit, setuid, setgid
	file.OpenPerm = strings.TrimPrefix(file.OpenPerm, "-")
	// file descriptors always have no permission for group and others
	file.OpenPerm, ok = strings.CutSuffix(file.OpenPerm, "------")
	if ok {
		// file descriptors always have execute permission
		file.OpenPerm = strings.TrimSuffix(file.OpenPerm, "x")
	}

	var inode uint64
	if file.Type, file.FilePerm, file.Size, inode = fileStats(ofl.stat, fdLinkPath); file.Type == "" {
		return File{}, false
	}

	if info, ok := ofl.socketInfo[inode]; ok {
		file.Name = info.Description
		file.FilePerm = info.State
		file.Type = info.Protocol
	} else {
		file.Name, err = ofl.readlink(fdLinkPath)
		if err != nil {
			return File{}, false
		}
	}

	return file, true
}

// TCP state codes, from the Linux kernel
// https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/net/tcp_states.h
//
// The same codes are used for UDP, but only ESTABLISHED and CLOSE are valid
var tcpStates = map[uint64]string{
	1:  "ESTABLISHED",
	2:  "SYN_SENT",
	3:  "SYN_RECV",
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
	11: "CLOSING",
	12: "NEW_SYN_RECV",
	13: "BOUND_INACTIVE",
}

func stateStr(state uint64) string {
	if s, ok := tcpStates[state]; ok {
		return s
	}
	return fmt.Sprintf("UNKNOWN(%d)", state)
}

// readSocketInfo reads the socket information from /proc/<pid>/net/{tcp,tcp6,udp,udp6,unix}
// returns a map of inode to socketInfo
// see https://www.kernel.org/doc/Documentation/networking/proc_net_tcp.txt
func readSocketInfo(procPIDPath string) map[uint64]socketInfo {
	si := make(map[uint64]socketInfo)

	fs, err := procfs.NewFS(procPIDPath)
	if err != nil {
		log.Debugf("Failed to read %s: %s", procPIDPath, err)
		return si
	}

	for protocol, parser := range map[string]func() (procfs.NetTCP, error){
		"tcp":  fs.NetTCP,
		"tcp6": fs.NetTCP6,
	} {
		addrs, err := parser()
		if err != nil {
			log.Debugf("Failed to read %s socket info in %s: %s", protocol, procPIDPath, err)
			continue
		}
		for _, entry := range addrs {
			si[entry.Inode] = socketInfo{
				fmt.Sprintf("%s:%d->%s:%d", entry.LocalAddr, entry.LocalPort, entry.RemAddr, entry.RemPort),
				stateStr(entry.St),
				protocol,
			}
		}
	}

	for protocol, parser := range map[string]func() (procfs.NetUDP, error){
		"udp":  fs.NetUDP,
		"udp6": fs.NetUDP6,
	} {
		addrs, err := parser()
		if err != nil {
			log.Debugf("Failed to read %s socket info in %s: %s", protocol, procPIDPath, err)
			continue
		}
		for _, entry := range addrs {
			si[entry.Inode] = socketInfo{
				fmt.Sprintf("%s:%d->%s:%d", entry.LocalAddr, entry.LocalPort, entry.RemAddr, entry.RemPort),
				stateStr(entry.St),
				protocol,
			}
		}
	}

	unix, err := fs.NetUNIX()
	if err == nil {
		for _, entry := range unix.Rows {
			si[entry.Inode] = socketInfo{
				fmt.Sprintf("%s:%s", entry.Type, entry.Path),
				fmt.Sprintf("%s:%s", entry.State, entry.Flags),
				"unix",
			}
		}
	} else {
		log.Debugf("Failed to read unix socket info in %s: %s", procPIDPath, err)
	}

	return si
}

func modeTypeToString(mode os.FileMode) string {
	switch mode & os.ModeType {
	case os.ModeSocket:
		return "SOCKET"
	case os.ModeNamedPipe:
		return "PIPE"
	case os.ModeDevice:
		return "DEV"
	case os.ModeDir:
		return "DIR"
	case os.ModeCharDevice:
		return "CHAR"
	case os.ModeSymlink:
		return "LINK"
	case os.ModeIrregular:
		return "?"
	default:
		return "REG"
	}
}

// fileStats returns the type, permission, size, and inode of a file
func fileStats(statf func(string) (os.FileInfo, error), path string) (string, string, int64, uint64) {
	stat, err := statf(path)
	if err != nil {
		log.Debugf("stat failed for %s: %v", path, err)
		return "", "", 0, 0
	}

	fileType := modeTypeToString(stat.Mode())

	size := stat.Size()
	perm := stat.Mode().Perm().String()

	var ino uint64
	// The inode number is not part of the exported interface of `os.FileInfo`,
	// so we need to use the underlying type to get it
	// `syscall.Stat_t` is the underlying type of `os.FileInfo.Sys()` on Linux
	// but type check to be safe and avoid unexpected panics
	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		ino = sys.Ino
	}

	return fileType, perm, size, ino
}

func procPath() string {
	if procPath, ok := os.LookupEnv("HOST_PROC"); ok {
		return procPath
	}
	return "/proc"
}
