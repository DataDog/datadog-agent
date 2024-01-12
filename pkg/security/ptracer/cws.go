// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	golog "log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/native"
)

func fillProcessCwd(process *Process) error {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", process.Pid))
	if err != nil {
		return err
	}
	process.Res.Cwd = cwd
	return nil
}

func getFullPathFromFd(process *Process, filename string, fd int32) (string, error) {
	if filename[0] != '/' {
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

func handleOpenAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	fd := tracer.ReadArgInt32(regs, 0)

	filename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFd(process, filename, fd)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeOpen
	msg.Open = &ebpfless.OpenSyscallMsg{
		Filename: filename,
		Flags:    uint32(tracer.ReadArgUint64(regs, 2)),
		Mode:     uint32(tracer.ReadArgUint64(regs, 3)),
	}

	return fillFileMetadata(filename, msg.Open, disableStats)
}

func handleOpen(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFilename(process, filename)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeOpen
	msg.Open = &ebpfless.OpenSyscallMsg{
		Filename: filename,
		Flags:    uint32(tracer.ReadArgUint64(regs, 1)),
		Mode:     uint32(tracer.ReadArgUint64(regs, 2)),
	}

	return fillFileMetadata(filename, msg.Open, disableStats)
}

func handleCreat(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFilename(process, filename)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeOpen
	msg.Open = &ebpfless.OpenSyscallMsg{
		Filename: filename,
		Flags:    unix.O_CREAT | unix.O_WRONLY | unix.O_TRUNC,
		Mode:     uint32(tracer.ReadArgUint64(regs, 1)),
	}

	return fillFileMetadata(filename, msg.Open, disableStats)
}

func handleMemfdCreate(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}
	filename = "memfd:" + filename

	msg.Type = ebpfless.SyscallTypeOpen
	msg.Open = &ebpfless.OpenSyscallMsg{
		Filename: filename,
		Flags:    uint32(tracer.ReadArgUint64(regs, 1)),
	}
	return nil
}

func handleNameToHandleAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	fd := tracer.ReadArgInt32(regs, 0)

	filename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFd(process, filename, fd)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeOpen
	msg.Open = &ebpfless.OpenSyscallMsg{
		Filename: filename,
	}
	return nil
}

func handleNameToHandleAtRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) {
	if msg.Open == nil {
		return
	}

	if ret := tracer.ReadRet(regs); ret < 0 {
		return
	}

	pFileHandleData, err := tracer.ReadArgData(process.Pid, regs, 2, 8 /*sizeof uint32 + sizeof int32*/)
	if err != nil {
		return
	}
	var handleBytes uint32
	var handleType int32
	buf := bytes.NewReader(pFileHandleData[:4])
	err = binary.Read(buf, native.Endian, &handleBytes)
	if err != nil {
		return
	}
	buf = bytes.NewReader(pFileHandleData[4:8])
	err = binary.Read(buf, native.Endian, &handleType)
	if err != nil {
		return
	}

	key := fileHandleKey{
		handleBytes: handleBytes,
		handleType:  handleType,
	}
	process.Res.FileHandleCache[key] = &fileHandleVal{
		pathName: msg.Open.Filename,
	}
}

func handleOpenByHandleAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	pFileHandleData, err := tracer.ReadArgData(process.Pid, regs, 1, 8 /*sizeof uint32 + sizeof int32*/)
	if err != nil {
		return err
	}
	var handleBytes uint32
	var handleType int32
	buf := bytes.NewReader(pFileHandleData[:4])
	err = binary.Read(buf, native.Endian, &handleBytes)
	if err != nil {
		return err
	}
	buf = bytes.NewReader(pFileHandleData[4:8])
	err = binary.Read(buf, native.Endian, &handleType)
	if err != nil {
		return err
	}

	key := fileHandleKey{
		handleBytes: handleBytes,
		handleType:  handleType,
	}
	val, ok := process.Res.FileHandleCache[key]
	if !ok {
		return errors.New("didn't find correspondance in the file handle cache")
	}
	msg.Type = ebpfless.SyscallTypeOpen
	msg.Open = &ebpfless.OpenSyscallMsg{
		Filename: val.pathName,
		Flags:    uint32(tracer.ReadArgUint64(regs, 2)),
	}
	return fillFileMetadata(val.pathName, msg.Open, disableStats)
}

func handleUnlinkat(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	fd := tracer.ReadArgInt32(regs, 0)

	filename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	flags := tracer.ReadArgInt32(regs, 2)

	filename, err = getFullPathFromFd(process, filename, fd)
	if err != nil {
		return err
	}

	if flags == unix.AT_REMOVEDIR {
		msg.Type = ebpfless.SyscallTypeRmdir
		msg.Rmdir = &ebpfless.RmdirSyscallMsg{
			File: ebpfless.OpenSyscallMsg{
				Filename: filename,
			},
		}
		err = fillFileMetadata(filename, &msg.Rmdir.File, disableStats)
	} else {
		msg.Type = ebpfless.SyscallTypeUnlink
		msg.Unlink = &ebpfless.UnlinkSyscallMsg{
			File: ebpfless.OpenSyscallMsg{
				Filename: filename,
			},
		}
		err = fillFileMetadata(filename, &msg.Unlink.File, disableStats)
	}
	return err
}

func handleUnlink(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFilename(process, filename)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeUnlink
	msg.Unlink = &ebpfless.UnlinkSyscallMsg{
		File: ebpfless.OpenSyscallMsg{
			Filename: filename,
		},
	}
	return fillFileMetadata(filename, &msg.Unlink.File, disableStats)
}

func handleRmdir(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFilename(process, filename)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeRmdir
	msg.Rmdir = &ebpfless.RmdirSyscallMsg{
		File: ebpfless.OpenSyscallMsg{
			Filename: filename,
		},
	}
	return fillFileMetadata(filename, &msg.Rmdir.File, disableStats)
}

func handleRename(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	oldFilename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	oldFilename, err = getFullPathFromFilename(process, oldFilename)
	if err != nil {
		return err
	}

	newFilename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	newFilename, err = getFullPathFromFilename(process, newFilename)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeRename
	msg.Rename = &ebpfless.RenameSyscallMsg{
		OldFile: ebpfless.OpenSyscallMsg{
			Filename: oldFilename,
		},
		NewFile: ebpfless.OpenSyscallMsg{
			Filename: newFilename,
		},
	}
	return fillFileMetadata(oldFilename, &msg.Rename.OldFile, disableStats)
}

func handleRenameAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	oldFD := tracer.ReadArgInt32(regs, 0)

	oldFilename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	oldFilename, err = getFullPathFromFd(process, oldFilename, oldFD)
	if err != nil {
		return err
	}

	newFD := tracer.ReadArgInt32(regs, 2)

	newFilename, err := tracer.ReadArgString(process.Pid, regs, 3)
	if err != nil {
		return err
	}

	newFilename, err = getFullPathFromFd(process, newFilename, newFD)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeRename
	msg.Rename = &ebpfless.RenameSyscallMsg{
		OldFile: ebpfless.OpenSyscallMsg{
			Filename: oldFilename,
		},
		NewFile: ebpfless.OpenSyscallMsg{
			Filename: newFilename,
		},
	}
	return fillFileMetadata(oldFilename, &msg.Rename.OldFile, disableStats)
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

func handleExecveAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	fd := tracer.ReadArgInt32(regs, 0)

	filename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	if filename == "" { // in this case, dirfd defines directly the file's FD
		var exists bool
		if filename, exists = process.Res.Fd[fd]; !exists || filename == "" {
			return errors.New("can't find related file path")
		}
	} else {
		filename, err = getFullPathFromFd(process, filename, fd)
		if err != nil {
			return err
		}
	}

	args, err := tracer.ReadArgStringArray(process.Pid, regs, 2)
	if err != nil {
		return err
	}
	args, argsTruncated := truncateArgs(args)

	envs, err := tracer.ReadArgStringArray(process.Pid, regs, 3)
	if err != nil {
		return err
	}
	envs, envsTruncated := truncateEnvs(envs)

	msg.Type = ebpfless.SyscallTypeExec
	msg.Exec = &ebpfless.ExecSyscallMsg{
		File: ebpfless.OpenSyscallMsg{
			Filename: filename,
		},
		Args:          args,
		ArgsTruncated: argsTruncated,
		Envs:          envs,
		EnvsTruncated: envsTruncated,
		TTY:           getPidTTY(process.Pid),
	}
	err = fillFileMetadata(filename, &msg.Exec.File, disableStats)
	if err != nil {
		return err
	}
	return nil
}

func handleFcntl(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeFcntl
	msg.Fcntl = &ebpfless.FcntlSyscallMsg{
		Fd:  tracer.ReadArgUint32(regs, 0),
		Cmd: tracer.ReadArgUint32(regs, 1),
	}
	return nil
}

func handleExecve(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFilename(process, filename)
	if err != nil {
		return err
	}

	args, err := tracer.ReadArgStringArray(process.Pid, regs, 1)
	if err != nil {
		return err
	}
	args, argsTruncated := truncateArgs(args)

	envs, err := tracer.ReadArgStringArray(process.Pid, regs, 2)
	if err != nil {
		return err
	}
	envs, envsTruncated := truncateEnvs(envs)

	msg.Type = ebpfless.SyscallTypeExec
	msg.Exec = &ebpfless.ExecSyscallMsg{
		File: ebpfless.OpenSyscallMsg{
			Filename: filename,
		},
		Args:          args,
		ArgsTruncated: argsTruncated,
		Envs:          envs,
		EnvsTruncated: envsTruncated,
		TTY:           getPidTTY(process.Pid),
	}
	err = fillFileMetadata(filename, &msg.Exec.File, disableStats)
	if err != nil {
		return err
	}
	return nil
}

func handleDup(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	// using msg to temporary store arg0, as it will be erased by the return value on ARM64
	msg.Dup = &ebpfless.DupSyscallFakeMsg{
		OldFd: tracer.ReadArgInt32(regs, 0),
	}
	return nil
}

func handleChdir(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	// using msg to temporary store arg0, as it will be erased by the return value on ARM64
	dirname, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	dirname, err = getFullPathFromFilename(process, dirname)
	if err != nil {
		process.Res.Cwd = ""
		return err
	}

	msg.Chdir = &ebpfless.ChdirSyscallFakeMsg{
		Path: dirname,
	}
	return nil
}

func handleFchdir(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	fd := tracer.ReadArgInt32(regs, 0)
	dirname, ok := process.Res.Fd[fd]
	if !ok {
		process.Res.Cwd = ""
		return nil
	}

	// using msg to temporary store arg0, as it will be erased by the return value on ARM64
	msg.Chdir = &ebpfless.ChdirSyscallFakeMsg{
		Path: dirname,
	}
	return nil
}

func handleSetuid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeSetUID
	msg.SetUID = &ebpfless.SetUIDSyscallMsg{
		UID:  tracer.ReadArgInt32(regs, 0),
		EUID: -1,
	}
	return nil
}

func handleSetgid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeSetGID
	msg.SetGID = &ebpfless.SetGIDSyscallMsg{
		GID:  tracer.ReadArgInt32(regs, 0),
		EGID: -1,
	}
	return nil
}

func handleSetreuid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeSetUID
	msg.SetUID = &ebpfless.SetUIDSyscallMsg{
		UID:  tracer.ReadArgInt32(regs, 0),
		EUID: tracer.ReadArgInt32(regs, 1),
	}
	return nil
}

func handleSetregid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeSetGID
	msg.SetGID = &ebpfless.SetGIDSyscallMsg{
		GID:  tracer.ReadArgInt32(regs, 0),
		EGID: tracer.ReadArgInt32(regs, 1),
	}
	return nil
}

func handleSetfsuid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeSetFSUID
	msg.SetFSUID = &ebpfless.SetFSUIDSyscallMsg{
		FSUID: tracer.ReadArgInt32(regs, 0),
	}
	return nil
}

func handleSetfsgid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	msg.Type = ebpfless.SyscallTypeSetFSGID
	msg.SetFSGID = &ebpfless.SetFSGIDSyscallMsg{
		FSGID: tracer.ReadArgInt32(regs, 0),
	}
	return nil
}

func handleClose(tracer *Tracer, process *Process, _ *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	fd := tracer.ReadArgInt32(regs, 0)
	delete(process.Res.Fd, fd)
	return nil
}

func handleCapset(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	pCaps, err := tracer.ReadArgData(process.Pid, regs, 1, 24 /*sizeof uint32 x3 x2*/)
	if err != nil {
		return err
	}
	var (
		tmp       uint32
		effective uint64
		permitted uint64
	)

	// extract low bytes of effective caps
	buf := bytes.NewReader(pCaps[:4])
	err = binary.Read(buf, native.Endian, &tmp)
	if err != nil {
		return err
	}
	effective = uint64(tmp)
	// extract high bytes of effective caps
	buf = bytes.NewReader(pCaps[12:16])
	err = binary.Read(buf, native.Endian, &tmp)
	if err != nil {
		return err
	}
	// merge them together
	effective |= uint64(tmp) << 32

	// extract low bytes of permitted caps
	buf = bytes.NewReader(pCaps[4:8])
	err = binary.Read(buf, native.Endian, &tmp)
	if err != nil {
		return err
	}
	permitted = uint64(tmp)
	// extract high bytes of permitted caps
	buf = bytes.NewReader(pCaps[16:20])
	err = binary.Read(buf, native.Endian, &tmp)
	if err != nil {
		return err
	}
	// merge them together
	permitted |= uint64(tmp) << 32

	msg.Type = ebpfless.SyscallTypeCapset
	msg.Capset = &ebpfless.CapsetSyscallMsg{
		Effective: uint64(effective),
		Permitted: uint64(permitted),
	}
	return nil
}

func checkEntryPoint(path string) (string, error) {
	name, err := exec.LookPath(path)
	if err != nil {
		return "", err
	}

	name, err = filepath.Abs(name)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(name)
	if err != nil {
		return "", err
	}

	if !info.Mode().IsRegular() {
		return "", errors.New("entrypoint not a regular file")
	}

	if info.Mode()&0111 == 0 {
		return "", errors.New("entrypoint not an executable")
	}

	return name, nil
}

func isAcceptedRetval(retval int64) bool {
	return retval < 0 && retval != -int64(syscall.EACCES) && retval != -int64(syscall.EPERM)
}

func initConn(probeAddr string, nbAttempts uint) (net.Conn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", probeAddr)
	if err != nil {
		return nil, err
	}

	var (
		client net.Conn
	)

	err = retry.Do(func() error {
		client, err = net.DialTCP("tcp", nil, tcpAddr)
		return err
	}, retry.Delay(time.Second), retry.Attempts(nbAttempts))
	if err != nil {
		return nil, err
	}
	return client, nil
}

func sendMsg(client net.Conn, msg *ebpfless.Message) error {
	data, err := msgpack.Marshal(msg)
	if err != nil {
		return fmt.Errorf("unable to marshal message: %v", err)
	}

	// write size
	var size [4]byte
	native.Endian.PutUint32(size[:], uint32(len(data)))
	if _, err = client.Write(size[:]); err != nil {
		return fmt.Errorf("unabled to send size: %v", err)
	}

	if _, err = client.Write(data); err != nil {
		return fmt.Errorf("unabled to send message: %v", err)
	}
	return nil
}

// StartCWSPtracer start the ptracer
func StartCWSPtracer(args []string, envs []string, probeAddr string, creds Creds, verbose bool, async bool, disableStats bool) error {
	if len(args) == 0 {
		return fmt.Errorf("an executable is required")
	}
	entry, err := checkEntryPoint(args[0])
	if err != nil {
		return err
	}

	logErrorf := golog.Printf
	logDebugf := func(fmt string, args ...any) {}
	if verbose {
		logDebugf = func(fmt string, args ...any) {
			golog.Printf(fmt, args...)
		}
	}

	logDebugf("Run %s %v [%s]", entry, args, os.Getenv("DD_CONTAINER_ID"))

	var (
		client      net.Conn
		clientReady = make(chan bool, 1)
		wg          sync.WaitGroup
	)

	if probeAddr != "" {
		logDebugf("connection to system-probe...")
		if async {
			go func() {
				client, err = initConn(probeAddr, 600)
				if err != nil {
					return
				}
				clientReady <- true
				logDebugf("connection to system-probe initiated!")
			}()
		} else {
			client, err = initConn(probeAddr, 120)
			if err != nil {
				return err
			}
			clientReady <- true
			logDebugf("connection to system-probe initiated!")
		}
	}

	containerID, err := getCurrentProcContainerID()
	if err != nil {
		logErrorf("Retrieve container ID from proc failed: %v\n", err)
	}
	containerCtx, err := newContainerContext(containerID)
	if err != nil {
		return err
	}

	opts := Opts{
		Syscalls: PtracedSyscalls,
		Creds:    creds,
	}

	tracer, err := NewTracer(entry, args, envs, opts)
	if err != nil {
		return err
	}

	var (
		msgChan   = make(chan *ebpfless.Message, 100000)
		traceChan = make(chan bool)
		stopChan  = make(chan bool, 1)
	)

	pc := NewProcessCache()

	// first process
	process := NewProcess(tracer.PID)
	pc.Add(tracer.PID, process)

	wg.Add(1)
	go func() {
		defer wg.Done()

		var seq uint64

		// start tracing
		traceChan <- true

		if probeAddr != "" {
		LOOP:
			// wait for the client to be ready of stopped
			for {
				select {
				case <-stopChan:
					return
				case <-clientReady:
					break LOOP
				}
			}
			defer client.Close()
		}

		for msg := range msgChan {
			msg.SeqNum = seq

			if probeAddr != "" {
				logDebugf("sending message: %s", msg)
				if err := sendMsg(client, msg); err != nil {
					logDebugf("%v", err)
				}
			} else {
				logDebugf("sending message: %s", msg)
			}
			seq++
		}
	}()

	send := func(msg *ebpfless.Message) {
		select {
		case msgChan <- msg:
		default:
			logErrorf("unable to send message")
		}
	}

	send(&ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			NSID:             getNSID(),
			ContainerContext: containerCtx,
			EntrypointArgs:   args,
		},
	})

	cb := func(cbType CallbackType, nr int, pid int, ppid int, regs syscall.PtraceRegs, waitStatus *syscall.WaitStatus) {
		process := pc.Get(pid)
		if process == nil {
			process = NewProcess(pid)
			pc.Add(pid, process)
		}

		sendSyscallMsg := func(msg *ebpfless.SyscallMsg) {
			if msg == nil {
				return
			}
			msg.PID = uint32(process.Tgid)
			msg.Timestamp = uint64(time.Now().UnixNano())
			send(&ebpfless.Message{
				Type:    ebpfless.MessageTypeSyscall,
				Syscall: msg,
			})
		}

		switch cbType {
		case CallbackPreType:
			syscallMsg := &ebpfless.SyscallMsg{}
			if nr == ExecveatNr {
				// special case: sometimes, execveat returns as execve, to handle that, we force
				// the msg to be put in ExecveNr
				process.Nr[ExecveNr] = syscallMsg
			} else {
				process.Nr[nr] = syscallMsg
			}

			switch nr {
			case OpenNr:
				if err := handleOpen(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle open: %v", err)
					return
				}
			case OpenatNr, Openat2Nr:
				if err := handleOpenAt(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle openat: %v", err)
					return
				}
			case CreatNr:
				if err = handleCreat(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle creat: %v", err)
					return
				}
			case NameToHandleAtNr:
				if err = handleNameToHandleAt(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle name_to_handle_at: %v", err)
					return
				}
			case OpenByHandleAtNr:
				if err = handleOpenByHandleAt(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle open_by_handle_at: %v", err)
					return
				}
			case ExecveNr:
				if err = handleExecve(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle execve: %v", err)
					return
				}

				// Top level pid, add creds. For the other PIDs the creds will be propagated at the probe side
				if process.Pid == tracer.PID {
					var uid, gid uint32

					if creds.UID != nil {
						uid = *creds.UID
					} else {
						uid = uint32(os.Getuid())
					}

					if creds.GID != nil {
						gid = *creds.GID
					} else {
						gid = uint32(os.Getgid())
					}

					syscallMsg.Exec.Credentials = &ebpfless.Credentials{
						UID:  uid,
						EUID: uid,
						GID:  gid,
						EGID: gid,
					}
				}

				// special case for exec since the pre reports the pid while the post reports the tgid
				if process.Pid != process.Tgid {
					pc.Add(process.Tgid, process)
				}
			case ExecveatNr:
				if err = handleExecveAt(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle execveat: %v", err)
					return
				}

				// special case for exec since the pre reports the pid while the post reports the tgid
				if process.Pid != process.Tgid {
					pc.Add(process.Tgid, process)
				}
			case FcntlNr:
				_ = handleFcntl(tracer, process, syscallMsg, regs)
			case DupNr, Dup2Nr, Dup3Nr:
				if err = handleDup(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle dup: %v", err)
					return
				}
			case ChdirNr:
				if err = handleChdir(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle chdir: %v", err)
					return
				}
			case FchdirNr:
				if err = handleFchdir(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetuidNr:
				if err = handleSetuid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetgidNr:
				if err = handleSetgid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetreuidNr, SetresuidNr:
				if err = handleSetreuid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetregidNr, SetresgidNr:
				if err = handleSetregid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetfsuidNr:
				if err = handleSetfsuid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetfsgidNr:
				if err = handleSetfsgid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case CloseNr:
				if err = handleClose(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle close: %v", err)
					return
				}
			case MemfdCreateNr:
				if err = handleMemfdCreate(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle memfd_create: %v", err)
					return
				}
			case CapsetNr:
				if err = handleCapset(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle capset: %v", err)
					return
				}
			case UnlinkNr:
				if err := handleUnlink(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle unlink: %v", err)
					return
				}
			case UnlinkatNr:
				if err := handleUnlinkat(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle unlinkat: %v", err)
					return
				}
			case RmdirNr:
				if err := handleRmdir(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle rmdir: %v", err)
					return
				}
			case RenameNr:
				if err := handleRename(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle rename: %v", err)
					return
				}
			case RenameAtNr, RenameAt2Nr:
				if err := handleRenameAt(tracer, process, syscallMsg, regs, disableStats); err != nil {
					logErrorf("unable to handle renameat: %v", err)
					return
				}
			}
		case CallbackPostType:
			switch nr {
			case CloseNr:
				// nothing to do
			case ExecveNr, ExecveatNr:
				sendSyscallMsg(process.Nr[ExecveNr]) // special case for execveat: we store the msg in execve bucket (see upper)

				// now the pid is the tgid
				process.Pid = process.Tgid
			case NameToHandleAtNr:
				syscallMsg, exists := process.Nr[nr]
				if !exists || syscallMsg.Open == nil {
					return
				}
				handleNameToHandleAtRet(tracer, process, syscallMsg, regs)
			case OpenNr, OpenatNr, CreatNr, OpenByHandleAtNr, MemfdCreateNr:
				if ret := tracer.ReadRet(regs); !isAcceptedRetval(ret) {
					syscallMsg, exists := process.Nr[nr]
					if !exists || syscallMsg.Open == nil {
						return
					}
					syscallMsg.Retval = ret

					sendSyscallMsg(syscallMsg)

					// maintain fd/path mapping
					process.Res.Fd[int32(ret)] = syscallMsg.Open.Filename
				}
			case SetuidNr, SetgidNr, SetreuidNr, SetregidNr, SetresuidNr, SetresgidNr, SetfsuidNr, SetfsgidNr:
				if ret := tracer.ReadRet(regs); ret == 0 || nr == SetfsuidNr || nr == SetfsgidNr {
					syscallMsg, exists := process.Nr[nr]
					if !exists {
						return
					}
					sendSyscallMsg(syscallMsg)
				}
			case CloneNr:
				if flags := tracer.ReadArgUint64(regs, 0); flags&uint64(unix.SIGCHLD) == 0 {
					pc.SetAsThreadOf(process, ppid)
				} else if parent := pc.Get(ppid); parent != nil {
					sendSyscallMsg(&ebpfless.SyscallMsg{
						Type: ebpfless.SyscallTypeFork,
						Fork: &ebpfless.ForkSyscallMsg{
							PPID: uint32(parent.Tgid),
						},
					})
				}
			case Clone3Nr:
				data, err := tracer.ReadArgData(process.Pid, regs, 0, 8 /*sizeof flags only*/)
				if err != nil {
					return
				}
				if flags := binary.NativeEndian.Uint64(data); flags&uint64(unix.SIGCHLD) == 0 {
					pc.SetAsThreadOf(process, ppid)
				} else if parent := pc.Get(ppid); parent != nil {
					sendSyscallMsg(&ebpfless.SyscallMsg{
						Type: ebpfless.SyscallTypeFork,
						Fork: &ebpfless.ForkSyscallMsg{
							PPID: uint32(parent.Tgid),
						},
					})
				}
			case ForkNr, VforkNr:
				if parent := pc.Get(ppid); parent != nil {
					sendSyscallMsg(&ebpfless.SyscallMsg{
						Type: ebpfless.SyscallTypeFork,
						Fork: &ebpfless.ForkSyscallMsg{
							PPID: uint32(parent.Tgid),
						},
					})
				}
			case FcntlNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists {
						return
					}

					// maintain fd/path mapping
					if syscallMsg.Fcntl.Cmd == unix.F_DUPFD || syscallMsg.Fcntl.Cmd == unix.F_DUPFD_CLOEXEC {
						if path, exists := process.Res.Fd[int32(syscallMsg.Fcntl.Fd)]; exists {
							process.Res.Fd[int32(ret)] = path
						}
					}
				}
			case DupNr, Dup2Nr, Dup3Nr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists {
						return
					}
					path, ok := process.Res.Fd[syscallMsg.Dup.OldFd]
					if ok {
						// maintain fd/path in case of dups
						process.Res.Fd[int32(ret)] = path
					}
				}
			case ChdirNr, FchdirNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists || syscallMsg.Chdir == nil {
						return
					}
					process.Res.Cwd = syscallMsg.Chdir.Path
				}
			case CapsetNr:
				if ret := tracer.ReadRet(regs); ret == 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists || syscallMsg.Capset == nil {
						return
					}
					syscallMsg.Retval = ret
					sendSyscallMsg(syscallMsg)
				}

			case UnlinkNr, UnlinkatNr, RmdirNr:
				if ret := tracer.ReadRet(regs); ret == 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists {
						return
					}
					syscallMsg.Retval = ret
					sendSyscallMsg(syscallMsg)
				}

			case RenameNr, RenameAtNr, RenameAt2Nr:
				if ret := tracer.ReadRet(regs); ret == 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists {
						return
					}
					syscallMsg.Retval = ret
					err := fillFileMetadata(syscallMsg.Rename.NewFile.Filename, &syscallMsg.Rename.NewFile, disableStats)
					if err == nil {
						sendSyscallMsg(syscallMsg)
					}
				}
			}
		case CallbackExitType:
			// send exit only for process not threads
			if process.Pid == process.Tgid && waitStatus != nil {
				exitCtx := &ebpfless.ExitSyscallMsg{}
				if waitStatus.Exited() {
					exitCtx.Cause = model.ExitExited
					exitCtx.Code = uint32(waitStatus.ExitStatus())
				} else if waitStatus.CoreDump() {
					exitCtx.Cause = model.ExitCoreDumped
					exitCtx.Code = uint32(waitStatus.Signal())
				} else if waitStatus.Signaled() {
					exitCtx.Cause = model.ExitSignaled
					exitCtx.Code = uint32(waitStatus.Signal())
				}
				sendSyscallMsg(&ebpfless.SyscallMsg{
					Type: ebpfless.SyscallTypeExit,
					Exit: exitCtx,
				})
			}

			pc.Remove(process)
		}
	}

	<-traceChan

	defer func() {
		// stop client and msg chan reader
		stopChan <- true
		close(msgChan)
		wg.Wait()
	}()

	if err := tracer.Trace(cb); err != nil {
		return err
	}

	// let a few queued message being send
	time.Sleep(time.Second)

	return nil
}
