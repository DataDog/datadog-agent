// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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

	"github.com/DataDog/gopsutil/process"
	"github.com/prometheus/procfs"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// snapshot uses procfs to retrieve information about the current process
func (pn *ProcessNode) snapshot(owner ActivityTreeOwner, shouldMergePaths bool, stats *ActivityTreeStats, newEvent func() *model.Event) error {
	// call snapshot for all the children of the current node
	for _, child := range pn.Children {
		if err := child.snapshot(owner, shouldMergePaths, stats, newEvent); err != nil {
			return err
		}
		// iterate slowly
		time.Sleep(50 * time.Millisecond)
	}

	// snapshot the current process
	p, err := process.NewProcess(int32(pn.Process.Pid))
	if err != nil {
		// the process doesn't exist anymore, ignore
		return nil
	}

	// snapshot files
	if owner.IsEventTypeValid(model.FileOpenEventType) {
		if err = pn.snapshotFiles(p, shouldMergePaths, stats, newEvent); err != nil {
			return err
		}
	}

	// snapshot sockets
	if owner.IsEventTypeValid(model.BindEventType) {
		if err = pn.snapshotBoundSockets(p, stats, newEvent); err != nil {
			return err
		}
	}
	return nil
}

func (pn *ProcessNode) snapshotFiles(p *process.Process, shouldMergePaths bool, stats *ActivityTreeStats, newEvent func() *model.Event) error {
	// list the files opened by the process
	fileFDs, err := p.OpenFiles()
	if err != nil {
		return err
	}

	var files []string
	for _, fd := range fileFDs {
		files = append(files, fd.Path)
	}

	// list the mmaped files of the process
	memoryMaps, err := p.MemoryMaps(false)
	if err != nil {
		return err
	}

	for _, mm := range *memoryMaps {
		if mm.Path != pn.Process.FileEvent.PathnameStr {
			files = append(files, mm.Path)
		}
	}

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
		if err != nil {
			evt.Open.File.PathnameStr = resolvedPath
		} else {
			evt.Open.File.PathnameStr = f
		}
		evt.Open.File.BasenameStr = path.Base(evt.Open.File.PathnameStr)
		evt.Open.File.FileFields.Mode = uint16(stat.Mode)
		evt.Open.File.FileFields.Inode = stat.Ino
		evt.Open.File.FileFields.UID = stat.Uid
		evt.Open.File.FileFields.GID = stat.Gid

		evt.Open.File.Mode = evt.Open.File.FileFields.Mode
		// TODO: add open flags by parsing `/proc/[pid]/fdinfo/fd` + O_RDONLY|O_CLOEXEC for the shared libs

		_, _ = pn.InsertFileEvent(&evt.Open.File, evt, Snapshot, stats, shouldMergePaths, false)
	}
	return nil
}

func (pn *ProcessNode) snapshotBoundSockets(p *process.Process, stats *ActivityTreeStats, newEvent func() *model.Event) error {
	// list all the file descriptors opened by the process
	FDs, err := p.OpenFiles()
	if err != nil {
		return err
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		if strings.HasPrefix(fd.Path, "socket:[") {
			sock, err := strconv.Atoi(strings.TrimPrefix(fd.Path[:len(fd.Path)-1], "socket:["))
			if err != nil {
				return err
			}
			if sock < 0 {
				continue
			}
			sockets = append(sockets, uint64(sock))
		}
	}
	if len(sockets) <= 0 {
		return nil
	}

	// use /proc/[pid]/net/tcp,tcp6,udp,udp6 to extract the ports opened by the current process
	proc, _ := procfs.NewFS(filepath.Join(util.HostProc(fmt.Sprintf("%d", p.Pid))))
	if err != nil {
		return err
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
	return nil
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

	_, _ = pn.InsertBindEvent(evt, Snapshot, stats, false)
}
