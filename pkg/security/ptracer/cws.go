// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"errors"
	"fmt"
	golog "log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
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
func StartCWSPtracer(args []string, probeAddr string, creds Creds, verbose bool, async bool) error {
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

	tracer, err := NewTracer(entry, args, opts)
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
					logErrorf("%v", err)
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

	cb := func(cbType CallbackType, nr int, pid int, ppid int, regs syscall.PtraceRegs) {
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
			send(&ebpfless.Message{
				Type:    ebpfless.MessageTypeSyscall,
				Syscall: msg,
			})
		}

		switch cbType {
		case CallbackPreType:
			syscallMsg := &ebpfless.SyscallMsg{}
			process.Nr[nr] = syscallMsg

			switch nr {
			case OpenNr:
				if err := handleOpen(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle open: %v", err)
					return
				}
			case OpenatNr, Openat2Nr:
				if err := handleOpenAt(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle openat: %v", err)
					return
				}
			case ExecveNr:
				if err = handleExecve(tracer, process, syscallMsg, regs); err != nil {
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
				if err = handleExecveAt(tracer, process, syscallMsg, regs); err != nil {
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
			case SetreuidNr:
				if err = handleSetreuid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			case SetregidNr:
				if err = handleSetregid(tracer, process, syscallMsg, regs); err != nil {
					logErrorf("unable to handle fchdir: %v", err)
					return
				}
			}
		case CallbackPostType:
			switch nr {
			case ExecveNr, ExecveatNr:
				sendSyscallMsg(process.Nr[nr])

				// now the pid is the tgid
				process.Pid = process.Tgid
			case OpenNr, OpenatNr:
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
			case SetuidNr, SetgidNr, SetreuidNr, SetregidNr:
				if ret := tracer.ReadRet(regs); ret >= 0 {
					syscallMsg, exists := process.Nr[nr]
					if !exists {
						return
					}

					sendSyscallMsg(syscallMsg)
				}
			case CloneNr:
				if flags := tracer.ReadArgUint64(regs, 0); flags&uint64(unix.SIGCHLD) == 0 {
					pc.SetAsThreadOf(process, ppid)
					return
				}
				fallthrough
			case ForkNr, VforkNr:
				sendSyscallMsg(&ebpfless.SyscallMsg{
					Type: ebpfless.SyscallTypeFork,
					Fork: &ebpfless.ForkSyscallMsg{
						PPID: uint32(ppid),
					},
				})
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
			}
		case CallbackExitType:
			// send exit only for process not threads
			if process.Pid == process.Tgid {
				sendSyscallMsg(&ebpfless.SyscallMsg{
					Type: ebpfless.SyscallTypeExit,
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
