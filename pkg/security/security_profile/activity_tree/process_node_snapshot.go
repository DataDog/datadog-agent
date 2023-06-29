// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activity_tree

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	legacyprocess "github.com/DataDog/gopsutil/process"
	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// snapshot uses procfs to retrieve information about the current process
func (pn *ProcessNode) snapshot(owner ActivityTreeOwner, stats *ActivityTreeStats, newEvent func() *model.Event, reducer *PathsReducer) {
	// call snapshot for all the children of the current node
	for _, child := range pn.Children {
		child.snapshot(owner, stats, newEvent, reducer)
		// iterate slowly
		time.Sleep(50 * time.Millisecond)
	}

	// snapshot the current process
	p, err := process.NewProcess(int32(pn.Process.Pid))
	if err != nil {
		// the process doesn't exist anymore, ignore
		return
	}

	// snapshot files
	if owner.IsEventTypeValid(model.FileOpenEventType) {
		pn.snapshotFiles(p, stats, newEvent, reducer)
	}

	// snapshot sockets
	if owner.IsEventTypeValid(model.BindEventType) {
		pn.snapshotBoundSockets(p, stats, newEvent)
	}
}

func (pn *ProcessNode) snapshotFiles(p *process.Process, stats *ActivityTreeStats, newEvent func() *model.Event, reducer *PathsReducer) {
	// list the files opened by the process
	fileFDs, err := p.OpenFiles()
	if err != nil {
		seclog.Warnf("error while listing files (pid: %v): %s", p.Pid, err)
	}

	var files []string
	for _, fd := range fileFDs {
		files = append(files, fd.Path)
	}

	// list the mmaped files of the process
	mmapedFiles, err := snapshotMemoryMappedFiles(p.Pid, pn.Process.FileEvent.PathnameStr)
	if err != nil {
		seclog.Warnf("error while listing memory maps (pid: %v): %s", p.Pid, err)
	}

	if len(files) == 0 {
		return
	}
	files = append(files, mmapedFiles...)

	// often the mmaped files are already nearly sorted, so we take the quick win and de-duplicate without sorting
	files = slices.Compact(files)

	// insert files
	var fileinfo os.FileInfo
	var resolvedPath string
	for _, f := range files {
		if len(f) == 0 {
			continue
		}

		// fetch the file user, group and mode
		fullPath := filepath.Join(utils.RootPath(int32(pn.Process.Pid)), f)
		fileinfo, err = os.Stat(fullPath)
		if err != nil {
			continue
		}
		stat, ok := fileinfo.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}

		evt := newEvent()
		evt.Type = uint32(model.FileOpenEventType)

		resolvedPath, err = filepath.EvalSymlinks(f)
		if err == nil && len(resolvedPath) != 0 {
			evt.Open.File.SetPathnameStr(resolvedPath)
		} else {
			evt.Open.File.SetPathnameStr(f)
		}
		evt.Open.File.SetBasenameStr(path.Base(evt.Open.File.PathnameStr))
		evt.Open.File.FileFields.Mode = uint16(stat.Mode)
		evt.Open.File.FileFields.Inode = stat.Ino
		evt.Open.File.FileFields.UID = stat.Uid
		evt.Open.File.FileFields.GID = stat.Gid

		evt.Open.File.Mode = evt.Open.File.FileFields.Mode
		// TODO: add open flags by parsing `/proc/[pid]/fdinfo/fd` + O_RDONLY|O_CLOEXEC for the shared libs

		_ = pn.InsertFileEvent(&evt.Open.File, evt, Snapshot, stats, false, reducer)
	}
}

const MAX_MMAPED_FILES = 128

func snapshotMemoryMappedFiles(pid int32, processEventPath string) ([]string, error) {
	fakeprocess := legacyprocess.Process{Pid: pid}
	stats, err := fakeprocess.MemoryMaps(false)
	if err != nil || stats == nil {
		return nil, err
	}

	files := make([]string, 0, MAX_MMAPED_FILES)
	for _, mm := range *stats {
		if len(files) >= MAX_MMAPED_FILES {
			break
		}

		if len(mm.Path) == 0 {
			continue
		}

		if mm.Path != processEventPath {
			continue
		}

		files = append(files, mm.Path)
	}
	return files, nil
}

func (pn *ProcessNode) snapshotBoundSockets(p *process.Process, stats *ActivityTreeStats, newEvent func() *model.Event) {
	// list all the file descriptors opened by the process
	FDs, err := p.OpenFiles()
	if err != nil {
		seclog.Warnf("error while listing files (pid: %v): %s", p.Pid, err)
		return
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		if strings.HasPrefix(fd.Path, "socket:[") {
			sock, err := strconv.Atoi(strings.TrimPrefix(fd.Path[:len(fd.Path)-1], "socket:["))
			if err != nil {
				seclog.Warnf("error while parsing socket inode (pid: %v): %s", p.Pid, err)
				continue
			}
			if sock < 0 {
				continue
			}
			sockets = append(sockets, uint64(sock))
		}
	}
	if len(sockets) <= 0 {
		return
	}

	// use /proc/[pid]/net/tcp,tcp6,udp,udp6 to extract the ports opened by the current process
	proc, _ := procfs.NewFS(filepath.Join(util.HostProc(fmt.Sprintf("%d", p.Pid))))
	if err != nil {
		seclog.Warnf("error while opening procfs (pid: %v): %s", p.Pid, err)
	}
	// looking for AF_INET sockets
	TCP, err := proc.NetTCP()
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP sockets: %v", err)
	}
	UDP, err := proc.NetUDP()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP sockets: %v", err)
	}
	// looking for AF_INET6 sockets
	TCP6, err := proc.NetTCP6()
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP6 sockets: %v", err)
	}
	UDP6, err := proc.NetUDP6()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP6 sockets: %v", err)
	}

	// searching for socket inode
	for _, s := range sockets {
		for _, sock := range TCP {
			if sock.Inode == s {
				pn.insertSnapshottedSocket(unix.AF_INET, sock.LocalAddr, uint16(sock.LocalPort), stats, newEvent)
				break
			}
		}
		for _, sock := range UDP {
			if sock.Inode == s {
				pn.insertSnapshottedSocket(unix.AF_INET, sock.LocalAddr, uint16(sock.LocalPort), stats, newEvent)
				break
			}
		}
		for _, sock := range TCP6 {
			if sock.Inode == s {
				pn.insertSnapshottedSocket(unix.AF_INET6, sock.LocalAddr, uint16(sock.LocalPort), stats, newEvent)
				break
			}
		}
		for _, sock := range UDP6 {
			if sock.Inode == s {
				pn.insertSnapshottedSocket(unix.AF_INET6, sock.LocalAddr, uint16(sock.LocalPort), stats, newEvent)
				break
			}
		}
		// not necessary found here, can be also another kind of socket (AF_UNIX, AF_NETLINK, etc)
	}
}

func (pn *ProcessNode) insertSnapshottedSocket(family uint16, ip net.IP, port uint16, stats *ActivityTreeStats, newEvent func() *model.Event) {
	evt := newEvent()
	evt.Type = uint32(model.BindEventType)

	evt.Bind.SyscallEvent.Retval = 0
	evt.Bind.AddrFamily = family
	evt.Bind.Addr.IPNet.IP = ip
	if family == unix.AF_INET {
		evt.Bind.Addr.IPNet.Mask = net.CIDRMask(32, 32)
	} else {
		evt.Bind.Addr.IPNet.Mask = net.CIDRMask(128, 128)
	}
	evt.Bind.Addr.Port = port

	_ = pn.InsertBindEvent(evt, Snapshot, stats, false)
}
