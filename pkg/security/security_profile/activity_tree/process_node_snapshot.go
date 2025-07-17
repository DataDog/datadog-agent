// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"bufio"
	"math/rand"
	"net"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// snapshot uses procfs to retrieve information about the current process
func (pn *ProcessNode) snapshot(owner Owner, stats *Stats, newEvent func() *model.Event, reducer *PathsReducer) {
	// call snapshot for all the children of the current node
	for _, child := range pn.Children {
		child.snapshot(owner, stats, newEvent, reducer)
	}

	// snapshot the current process
	p, err := process.NewProcess(int32(pn.Process.Pid))
	if err != nil {
		// the process doesn't exist anymore, ignore
		return
	}

	// snapshot files
	if owner.IsEventTypeValid(model.FileOpenEventType) {
		pn.snapshotAllFiles(p, stats, newEvent, reducer)
	}

	// snapshot sockets
	if owner.IsEventTypeValid(model.BindEventType) {
		pn.snapshotBoundSockets(p, stats, newEvent)
	}
}

// maxFDsPerProcessSnapshot represents the maximum number of FDs we will collect per process while snapshotting
// this value was selected because it represents the default upper bound for the number of FDs a linux process can have
const maxFDsPerProcessSnapshot = 1024

func (pn *ProcessNode) snapshotAllFiles(p *process.Process, stats *Stats, newEvent func() *model.Event, reducer *PathsReducer) {
	// list the files opened by the process
	fileFDs, err := p.OpenFiles()
	if err != nil {
		seclog.Warnf("error while listing files (pid: %v): %s", p.Pid, err)
	}

	// filter out fd corresponding to anon inodes, pipes, sockets, etc. when snapshotting process opened files
	// the goal is to avoid sampling opened files for processes that mostly open files not present on the filesystem
	fileFDs = slices.DeleteFunc(fileFDs, func(fd process.OpenFilesStat) bool {
		return !strings.HasPrefix(fd.Path, "/")
	})

	var (
		isSampling = false
		preAlloc   = len(fileFDs)
	)

	if len(fileFDs) > maxFDsPerProcessSnapshot {
		isSampling = true
		preAlloc = 1024
	}

	files := make([]string, 0, preAlloc)
	for _, fd := range fileFDs {
		if len(files) >= maxFDsPerProcessSnapshot {
			break
		}

		if !isSampling || rand.Int63n(int64(len(fileFDs))) < maxFDsPerProcessSnapshot {
			files = append(files, fd.Path)
		}
	}
	if isSampling {
		seclog.Warnf("sampled open files while snapshotting (pid: %v): kept %d of %d files", p.Pid, len(files), len(fileFDs))
	}

	// list the mmaped files of the process
	mmapedFiles, err := getMemoryMappedFiles(p.Pid, pn.Process.FileEvent.PathnameStr)
	if err != nil {
		seclog.Warnf("error while listing memory maps (pid: %v): %s", p.Pid, err)
	}

	files = append(files, mmapedFiles...)
	if len(files) == 0 {
		return
	}

	pn.addFiles(files, stats, newEvent, reducer)
}

func (pn *ProcessNode) addFiles(files []string, stats *Stats, newEvent func() *model.Event, reducer *PathsReducer) {
	// list the mmaped files of the process
	slices.Sort(files)
	files = slices.Compact(files)

	// insert files
	var (
		err          error
		resolvedPath string
	)
	for _, f := range files {
		if len(f) == 0 {
			continue
		}

		evt := newEvent()
		fullPath := utils.ProcRootFilePath(pn.Process.Pid, f)
		if evt.ProcessContext == nil {
			evt.ProcessContext = &model.ProcessContext{}
		}
		if evt.ContainerContext == nil {
			evt.ContainerContext = &model.ContainerContext{}
		}
		evt.ProcessContext.Process = pn.Process
		evt.CGroupContext.CGroupID = containerutils.CGroupID(pn.Process.CGroup.CGroupID)
		evt.CGroupContext.CGroupFlags = pn.Process.CGroup.CGroupFlags
		evt.ContainerContext.ContainerID = containerutils.ContainerID(pn.Process.ContainerID)

		var fileStats unix.Statx_t
		if err := unix.Statx(unix.AT_FDCWD, fullPath, 0, unix.STATX_ALL, &fileStats); err != nil {
			stat, err := utils.UnixStat(fullPath)
			if err != nil {
				seclog.Tracef("unable to stat mapped file %s", fullPath)
				continue
			}

			evt.Open.File.FileFields.Mode = uint16(stat.Mode)
			evt.Open.File.FileFields.Inode = stat.Ino
			evt.Open.File.FileFields.UID = stat.Uid
			evt.Open.File.FileFields.GID = stat.Gid

			mode := utils.UnixStatModeToGoFileMode(stat.Mode)
			if mode.IsRegular() {
				evt.FieldHandlers.ResolveHashes(model.FileOpenEventType, &pn.Process, &evt.Open.File)
			}
		} else {
			evt.Open.File.FileFields.Mode = uint16(fileStats.Mode)
			evt.Open.File.FileFields.Inode = fileStats.Ino
			evt.Open.File.FileFields.UID = fileStats.Uid
			evt.Open.File.FileFields.GID = fileStats.Gid

			evt.Open.File.CTime = uint64(time.Unix(fileStats.Ctime.Sec, int64(fileStats.Ctime.Nsec)).Nanosecond())
			evt.Open.File.MTime = uint64(time.Unix(fileStats.Mtime.Sec, int64(fileStats.Mtime.Nsec)).Nanosecond())
			evt.Open.File.Mode = fileStats.Mode
			evt.Open.File.Inode = fileStats.Ino
			evt.Open.File.Device = fileStats.Dev_major<<20 | fileStats.Dev_minor
			evt.Open.File.NLink = fileStats.Nlink
			evt.Open.File.MountID = uint32(fileStats.Mnt_id)

			if (fileStats.Mode & syscall.S_IFREG) != 0 {
				evt.FieldHandlers.ResolveHashes(model.FileOpenEventType, &pn.Process, &evt.Open.File)
			}
		}

		evt.Type = uint32(model.FileOpenEventType)
		evt.Open.File.Mode = evt.Open.File.FileFields.Mode

		resolvedPath, err = filepath.EvalSymlinks(f)
		if err == nil && len(resolvedPath) != 0 {
			evt.Open.File.SetPathnameStr(resolvedPath)
		} else {
			evt.Open.File.SetPathnameStr(f)
		}
		evt.Open.File.SetBasenameStr(path.Base(evt.Open.File.PathnameStr))

		evt.FieldHandlers.ResolvePackageName(evt, &evt.Open.File)
		evt.FieldHandlers.ResolvePackageVersion(evt, &evt.Open.File)

		// TODO: add open flags by parsing `/proc/[pid]/fdinfo/fd` + O_RDONLY|O_CLOEXEC for the shared libs

		_ = pn.InsertFileEvent(&evt.Open.File, evt, "", Snapshot, stats, false, reducer, nil)
	}
}

// MaxMmapedFiles defines the max mmaped files
const MaxMmapedFiles = 128

func getMemoryMappedFiles(pid int32, processEventPath string) (files []string, _ error) {
	smapsPath := kernel.HostProc(strconv.Itoa(int(pid)), "smaps")
	smapsFile, err := os.Open(smapsPath)
	if err != nil {
		return nil, err
	}
	defer smapsFile.Close()

	files = make([]string, 0, MaxMmapedFiles)
	scanner := bufio.NewScanner(smapsFile)

	for scanner.Scan() && len(files) < MaxMmapedFiles {
		line := scanner.Bytes()

		path, ok := extractPathFromSmapsLine(line)
		if !ok {
			continue
		}

		if len(path) == 0 {
			continue
		}

		if path == processEventPath {
			continue
		}

		// skip [vdso], [stack], [heap] and similar mappings
		if strings.HasPrefix(path, "[") {
			continue
		}

		files = append(files, path)
	}

	return files, scanner.Err()
}

func extractPathFromSmapsLine(line []byte) (string, bool) {
	inSpace := false
	spaceCount := 0
	for i, c := range line {
		if c == ' ' || c == '\t' {
			// check for fields separator
			if !inSpace && spaceCount == 0 && i > 0 {
				if line[i-1] == ':' {
					return "", false
				}
			}

			if !inSpace {
				inSpace = true
				spaceCount++
			}
		} else if spaceCount == 5 {
			return string(line[i:]), true
		} else {
			inSpace = false
		}
	}
	return "", false
}

func (pn *ProcessNode) snapshotBoundSockets(p *process.Process, stats *Stats, newEvent func() *model.Event) {
	boundSockets, err := procfs.NewBoundSocketSnapshotter().GetBoundSockets(p)
	if err != nil {
		seclog.Warnf("error while listing sockets (pid: %v): %s", p.Pid, err)
		return
	}

	for _, socket := range boundSockets {
		pn.insertSnapshottedSocket(socket.Family, socket.IP, socket.Protocol, socket.Port, stats, newEvent)
	}

}

func (pn *ProcessNode) insertSnapshottedSocket(family uint16, ip net.IP, protocol uint16, port uint16, stats *Stats, newEvent func() *model.Event) {
	evt := newEvent()
	evt.Type = uint32(model.BindEventType)

	evt.Bind.SyscallEvent.Retval = 0
	evt.Bind.AddrFamily = family
	evt.Bind.Addr.IPNet.IP = ip
	evt.Bind.Protocol = protocol
	if family == unix.AF_INET {
		evt.Bind.Addr.IPNet.Mask = net.CIDRMask(32, 32)
	} else {
		evt.Bind.Addr.IPNet.Mask = net.CIDRMask(128, 128)
	}
	evt.Bind.Addr.Port = port

	_ = pn.InsertBindEvent(evt, "", Snapshot, stats, false)
}
