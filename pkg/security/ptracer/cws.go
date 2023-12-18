// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	golog "log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/util/native"
)

// Process represents a process context
type Process struct {
	Pid int
	Nr  map[int]*ebpfless.SyscallMsg
	Fd  map[int32]string
	Cwd string
}

func fillProcessCwd(process *Process) error {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", process.Pid))
	if err != nil {
		return err
	}
	process.Cwd = cwd
	return nil
}

func getFullPathFromFd(process *Process, filename string, fd int32) (string, error) {
	if filename[0] != '/' {
		if fd == unix.AT_FDCWD { // if use current dir, try to prefix it
			if process.Cwd != "" || fillProcessCwd(process) == nil {
				filename = filepath.Join(process.Cwd, filename)
			} else {
				return "", errors.New("fillProcessCwd failed")
			}
		} else { // if using another dir, prefix it, we should have it in cache
			if path, exists := process.Fd[fd]; exists {
				filename = filepath.Join(path, filename)
			} else {
				return "", errors.New("Process FD cache incomplete during path resolution")
			}
		}
	}
	return filename, nil
}

func getFullPathFromFilename(process *Process, filename string) (string, error) {
	if filename[0] != '/' {
		if process.Cwd != "" || fillProcessCwd(process) == nil {
			filename = filepath.Join(process.Cwd, filename)
		} else {
			return "", errors.New("fillProcessCwd failed")
		}
	}
	return filename, nil
}

func handleOpenAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
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

	return nil
}

func handleOpen(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
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

	return nil
}

func handleExecveAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	fd := tracer.ReadArgInt32(regs, 0)

	filename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFd(process, filename, fd)
	if err != nil {
		return err
	}

	args, err := tracer.ReadArgStringArray(process.Pid, regs, 2)
	if err != nil {
		return err
	}

	envs, err := tracer.ReadArgStringArray(process.Pid, regs, 3)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeExec
	msg.Exec = &ebpfless.ExecSyscallMsg{
		Filename: filename,
		Args:     args,
		Envs:     envs,
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

func handleExecve(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
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

	envs, err := tracer.ReadArgStringArray(process.Pid, regs, 2)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeExec
	msg.Exec = &ebpfless.ExecSyscallMsg{
		Filename: filename,
		Args:     args,
		Envs:     envs,
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
		process.Cwd = ""
		return err
	}

	msg.Chdir = &ebpfless.ChdirSyscallFakeMsg{
		Path: dirname,
	}
	return nil
}

func handleFchdir(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs) error {
	fd := tracer.ReadArgInt32(regs, 0)
	dirname, ok := process.Fd[fd]
	if !ok {
		process.Cwd = ""
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
		GID: tracer.ReadArgInt32(regs, 0),
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

// ECSMetadata defines ECS metadatas
type ECSMetadata struct {
	DockerID   string `json:"DockerId"`
	DockerName string `json:"DockerName"`
	Name       string `json:"Name"`
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

func retrieveECSMetadata(ctx *ebpfless.ContainerContext) error {
	url := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if url == "" {
		return nil
	}

	body, err := simpleHTTPRequest(url)
	if err != nil {
		return err
	}

	data := ECSMetadata{}
	if err = json.Unmarshal(body, &data); err != nil {
		return err
	}

	if data.DockerID != "" && ctx.ID == "" {
		// only set the container ID if we previously failed to retrieve it from proc
		ctx.ID = data.DockerID
	}
	if data.DockerName != "" {
		ctx.Name = data.DockerName
	}

	return nil
}

func retrieveEnvMetadata(ctx *ebpfless.ContainerContext) {
	if id := os.Getenv("DD_CONTAINER_ID"); id != "" {
		ctx.ID = id
	}

	if name := os.Getenv("DD_CONTAINER_NAME"); name != "" {
		ctx.Name = name
	}
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

// StartCWSPtracer start the ptracer
func StartCWSPtracer(args []string, probeAddr string, creds Creds, verbose bool) error {
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
		client net.Conn
	)

	if probeAddr != "" {
		tcpAddr, err := net.ResolveTCPAddr("tcp", probeAddr)
		if err != nil {
			return err
		}

		logDebugf("connection to system-probe...")

		err = retry.Do(func() error {
			client, err = net.DialTCP("tcp", nil, tcpAddr)
			return err
		}, retry.Delay(time.Second), retry.Attempts(120))
		if err != nil {
			return err
		}

		defer client.Close()
	}

	var containerCtx ebpfless.ContainerContext
	if err := retrieveContainerIDFromProc(&containerCtx); err != nil {
		logErrorf("Retrieve container ID from proc failed: %v\n", err)
	}
	if err := retrieveECSMetadata(&containerCtx); err != nil {
		return err
	}
	retrieveEnvMetadata(&containerCtx)
	containerCtx.CreatedAt = uint64(time.Now().UnixNano())

	opts := Opts{
		Syscalls: PtracedSyscalls,
		Creds:    creds,
	}

	tracer, err := NewTracer(entry, args, opts)
	if err != nil {
		return err
	}

	nsid := getNSID()
	msgChan := make(chan *ebpfless.SyscallMsg, 10000)
	traceChan := make(chan bool)

	cache, err := lru.New[int, *Process](1024)
	if err != nil {
		return err
	}

	// first process
	process := &Process{
		Pid: tracer.PID,
		Nr:  make(map[int]*ebpfless.SyscallMsg),
		Fd:  make(map[int32]string),
	}
	cache.Add(tracer.PID, process)

	go func() {
		var seq uint64

		traceChan <- true

		for msg := range msgChan {
			msg.SeqNum = seq
			msg.NSID = nsid

			logDebugf("sending message: %s", msg)

			if probeAddr != "" {
				data, err := msgpack.Marshal(msg)
				if err != nil {
					logErrorf("unable to marshal message: %v", err)
					return
				}

				// write size
				var size [4]byte
				native.Endian.PutUint32(size[:], uint32(len(data)))
				if _, err = client.Write(size[:]); err != nil {
					logErrorf("unabled to send size: %v", err)
				}

				if _, err = client.Write(data); err != nil {
					logErrorf("unabled to send message: %v", err)
				}
			}
			seq++
		}
	}()

	send := func(msg *ebpfless.SyscallMsg) {
		if msg == nil {
			return
		}

		select {
		case msgChan <- msg:
		default:
			logErrorf("unable to send message")
		}
	}

	cb := func(cbType CallbackType, nr int, pid int, ppid int, regs syscall.PtraceRegs) {
		process, exists := cache.Get(pid)
		if !exists {
			process = &Process{
				Pid: pid,
				Nr:  make(map[int]*ebpfless.SyscallMsg),
				Fd:  make(map[int32]string),
			}

			cache.Add(pid, process)
		}

		switch cbType {
		case CallbackPreType:
			msg := &ebpfless.SyscallMsg{
				PID:              uint32(pid),
				ContainerContext: &containerCtx,
			}
			process.Nr[nr] = msg

			switch nr {
			case OpenNr:
				if err := handleOpen(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle open: %v", err)
					return
				}
			case OpenatNr, Openat2Nr:
				if err := handleOpenAt(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle openat: %v", err)
					return
				}
			case ExecveNr:
				if err = handleExecve(tracer, process, msg, regs); err != nil {
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

					msg.Exec.Credentials = &ebpfless.Credentials{
						UID:  uid,
						EUID: uid,
						GID:  gid,
						EGID: gid,
					}
				}
			case ExecveatNr:
				if err = handleExecveAt(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle execveat: %v", err)
					return
				}
			case FcntlNr:
				_ = handleFcntl(tracer, process, msg, regs)
			case DupNr, Dup2Nr, Dup3Nr:
				if err = handleDup(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle dup: %v", err)
					return
				}
			case ChdirNr:
				if err = handleChdir(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle chdir: %v", err)
					return
				}
			case FchdirNr:
				if err = handleFchdir(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetuidNr:
				if err = handleSetuid(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetgidNr:
				if err = handleSetgid(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetreuidNr:
				if err = handleSetreuid(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetregidNr:
				if err = handleSetregid(tracer, process, msg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			}
		case CallbackPostType:
			switch nr {
			case ExecveNr, ExecveatNr:
				send(process.Nr[nr])
			case OpenNr, OpenatNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					msg, exists := process.Nr[nr]
					if !exists {
						return
					}

					send(msg)

					// maintain fd/path mapping
					process.Fd[int32(ret)] = msg.Open.Filename
				}
			case SetuidNr, SetgidNr, SetreuidNr, SetregidNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					msg, exists := process.Nr[nr]
					if !exists {
						return
					}

					send(msg)
				}
			case ForkNr, VforkNr, CloneNr:
				msg := &ebpfless.SyscallMsg{
					ContainerContext: &containerCtx,
				}
				msg.Type = ebpfless.SyscallTypeFork
				msg.PID = uint32(pid)
				msg.Fork = &ebpfless.ForkSyscallMsg{
					PPID: uint32(ppid),
				}
				send(msg)
			case FcntlNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					msg, exists := process.Nr[nr]
					if !exists {
						return
					}

					// maintain fd/path mapping
					if msg.Fcntl.Cmd == unix.F_DUPFD || msg.Fcntl.Cmd == unix.F_DUPFD_CLOEXEC {
						if path, exists := process.Fd[int32(msg.Fcntl.Fd)]; exists {
							process.Fd[int32(ret)] = path
						}
					}
				}
			case DupNr, Dup2Nr, Dup3Nr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					msg, exists := process.Nr[nr]
					if !exists {
						return
					}
					path, ok := process.Fd[msg.Dup.OldFd]
					if ok {
						// maintain fd/path in case of dups
						process.Fd[int32(ret)] = path
					}
				}
			case ChdirNr, FchdirNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					msg, exists := process.Nr[nr]
					if !exists || msg.Chdir == nil {
						return
					}
					process.Cwd = msg.Chdir.Path
				}
			}
		case CallbackExitType:
			msg := &ebpfless.SyscallMsg{
				ContainerContext: &containerCtx,
			}
			msg.Type = ebpfless.SyscallTypeExit
			msg.PID = uint32(pid)
			send(msg)

			cache.Remove(pid)
		}
	}

	<-traceChan

	if err := tracer.Trace(cb); err != nil {
		return err
	}

	// let a few queued message being send
	time.Sleep(time.Second)

	return nil
}
