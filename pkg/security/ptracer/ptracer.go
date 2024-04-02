// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"syscall"
	"time"

	"github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/util/native"
)

// CallbackType represents a callback type
type CallbackType = int

const (
	// CallbackPreType defines a callback called in pre stage
	CallbackPreType CallbackType = iota
	// CallbackPostType defines a callback called in post stage
	CallbackPostType
	// CallbackExitType defines a callback called at exit
	CallbackExitType

	// MaxStringSize defines the max read size
	MaxStringSize = 4096

	ptraceFlags = 0 |
		syscall.PTRACE_O_TRACEVFORK |
		syscall.PTRACE_O_TRACEFORK |
		syscall.PTRACE_O_TRACECLONE |
		syscall.PTRACE_O_TRACEEXEC |
		syscall.PTRACE_O_TRACESYSGOOD |
		unix.PTRACE_O_TRACESECCOMP

	defaultUserGroupRateLimit = time.Second
)

// Tracer represents a tracer
type Tracer struct {
	// PID represents a PID
	PID int

	// internals
	info *arch.Info
	opts TracerOpts
	// user and group cache
	// TODO: user opens of passwd/group files to reset the limiters?
	userCache                map[int]string
	userCacheRefreshLimiter  *rate.Limiter
	lastPasswdMTime          uint64
	groupCache               map[int]string
	groupCacheRefreshLimiter *rate.Limiter
	lastGroupMTime           uint64
}

// Creds defines credentials
type Creds struct {
	UID *uint32
	GID *uint32
}

// TracerOpts defines ptracer options
type TracerOpts struct {
	Syscalls []string
	Creds    Creds
	Logger   Logger
}

func processVMReadv(pid int, addr uintptr, data []byte) (int, error) {
	size := len(data)

	localIov := []unix.Iovec{
		{Base: &data[0], Len: uint64(size)},
	}

	remoteIov := []unix.RemoteIovec{
		{Base: addr, Len: size},
	}

	return unix.ProcessVMReadv(pid, localIov, remoteIov, 0)
}

func (t *Tracer) readString(pid int, ptr uint64) (string, error) {
	data := make([]byte, MaxStringSize)

	_, err := processVMReadv(pid, uintptr(ptr), data)
	if err != nil {
		return "", err
	}

	n := bytes.Index(data[:], []byte{0})
	if n < 0 {
		return "", nil
	}
	return string(data[:n]), nil
}

func (t *Tracer) readInt32(pid int, ptr uint64) (int32, error) {
	data := make([]byte, 4)

	_, err := processVMReadv(pid, uintptr(ptr), data)
	if err != nil {
		return 0, err
	}

	// []byte to int32
	buf := bytes.NewReader(data)
	var val int32
	err = binary.Read(buf, native.Endian, &val)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (t *Tracer) readData(pid int, ptr uint64, size uint) ([]byte, error) {
	data := make([]byte, size)

	_, err := processVMReadv(pid, uintptr(ptr), data)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

// PeekString peeks and returns a string from a pid at a given addr ptr
func (t *Tracer) PeekString(pid int, ptr uint64) (string, error) {
	var (
		result []byte
		data   = make([]byte, 1)
		i      uint64
	)

	for {
		n, err := syscall.PtracePeekData(pid, uintptr(ptr+i), data)
		if err != nil || n != len(data) {
			return "", err
		}
		if data[0] == 0 {
			break
		}

		result = append(result, data[0])

		i += uint64(len(data))
	}

	return string(result), nil
}

// ReadArgUint64 reads the regs and returns the wanted arg as uint64
func (t *Tracer) ReadArgUint64(regs syscall.PtraceRegs, arg int) uint64 {
	return t.argToRegValue(regs, arg)
}

// ReadArgInt64 reads the regs and returns the wanted arg as int64
func (t *Tracer) ReadArgInt64(regs syscall.PtraceRegs, arg int) int64 {
	return int64(t.argToRegValue(regs, arg))
}

// ReadArgInt32 reads the regs and returns the wanted arg as int32
func (t *Tracer) ReadArgInt32(regs syscall.PtraceRegs, arg int) int32 {
	return int32(t.argToRegValue(regs, arg))
}

// ReadArgInt32Ptr reads the regs and returns the wanted arg as int32
func (t *Tracer) ReadArgInt32Ptr(pid int, regs syscall.PtraceRegs, arg int) (int32, error) {
	ptr := t.argToRegValue(regs, arg)
	return t.readInt32(pid, ptr)
}

// ReadArgData reads the regs and returns the wanted arg as byte array
func (t *Tracer) ReadArgData(pid int, regs syscall.PtraceRegs, arg int, size uint) ([]byte, error) {
	ptr := t.argToRegValue(regs, arg)
	return t.readData(pid, ptr, size)
}

// ReadArgUint32 reads the regs and returns the wanted arg as uint32
func (t *Tracer) ReadArgUint32(regs syscall.PtraceRegs, arg int) uint32 {
	return uint32(t.argToRegValue(regs, arg))
}

// ReadArgString reads the regs and returns the wanted arg as string
func (t *Tracer) ReadArgString(pid int, regs syscall.PtraceRegs, arg int) (string, error) {
	ptr := t.argToRegValue(regs, arg)
	return t.readString(pid, ptr)
}

// GetSyscallName returns the given syscall name
func (t *Tracer) GetSyscallName(regs syscall.PtraceRegs) string {
	return t.info.SyscallNumbers[GetSyscallNr(regs)]
}

// ReadArgStringArray reads and returns the wanted arg as string array
func (t *Tracer) ReadArgStringArray(pid int, regs syscall.PtraceRegs, arg int) ([]string, error) {
	ptr := t.argToRegValue(regs, arg)

	var (
		result []string
		data   = make([]byte, 8)
		i      uint64
	)

	for {
		n, err := syscall.PtracePeekData(pid, uintptr(ptr+i), data)
		if err != nil || n != len(data) {
			return result, err
		}

		ptr := native.Endian.Uint64(data)
		if ptr == 0 {
			break
		}

		str, err := t.readString(pid, ptr)
		if err != nil {
			break
		}
		result = append(result, str)

		i += uint64(len(data))
	}

	return result, nil
}

// Trace traces a process
func (t *Tracer) Trace(cb func(cbType CallbackType, nr int, pid int, ppid int, regs syscall.PtraceRegs, waitStatus *syscall.WaitStatus)) error {
	var waitStatus syscall.WaitStatus

	if err := syscall.PtraceCont(t.PID, 0); err != nil {
		return err
	}

	var (
		regs   syscall.PtraceRegs
		prevNr int
	)

	for {
		pid, err := syscall.Wait4(-1, &waitStatus, 0, nil)
		if err != nil {
			t.opts.Logger.Debugf("unable to wait for pid %d: %v", pid, err)
			break
		}

		if waitStatus.Exited() || waitStatus.CoreDump() || waitStatus.Signaled() {
			if pid == t.PID {
				break
			}
			cb(CallbackExitType, ExitNr, pid, 0, regs, &waitStatus)
			continue
		}

		if waitStatus.Stopped() {
			if signal := waitStatus.StopSignal(); signal != syscall.SIGTRAP {
				if signal == syscall.SIGSTOP {
					signal = syscall.Signal(0)
				}
				if err := syscall.PtraceCont(pid, int(signal)); err == nil {
					continue
				}
			}

			if err := syscall.PtraceGetRegs(pid, &regs); err != nil {
				t.opts.Logger.Debugf("unable to get registers for pid %d: %v", pid, err)

				// it got probably killed
				cb(CallbackExitType, ExitNr, pid, 0, regs, &waitStatus)

				if pid == t.PID {
					break
				}
				continue
			}

			nr := GetSyscallNr(regs)
			if nr == 0 {
				nr = prevNr
			}
			prevNr = nr

			switch waitStatus.TrapCause() {
			case syscall.PTRACE_EVENT_CLONE, syscall.PTRACE_EVENT_FORK, syscall.PTRACE_EVENT_VFORK:
				if npid, err := syscall.PtraceGetEventMsg(pid); err == nil {
					cb(CallbackPostType, nr, int(npid), pid, regs, nil)
				}
			case syscall.PTRACE_EVENT_EXEC:
				cb(CallbackPostType, ExecveNr, pid, 0, regs, nil)
			case unix.PTRACE_EVENT_SECCOMP:
				switch nr {
				case ForkNr, VforkNr, CloneNr, Clone3Nr:
					// already handled
				default:
					cb(CallbackPreType, nr, pid, 0, regs, nil)

					// force a ptrace syscall in order to get to return value
					if err := syscall.PtraceSyscall(pid, 0); err != nil {
						t.opts.Logger.Debugf("unable to call ptrace syscall for pid %d: %v", pid, err)
					}
					continue
				}
			default:
				switch nr {
				case ForkNr, VforkNr, CloneNr, Clone3Nr:
					// already handled
				case ExecveNr, ExecveatNr:
					// triggered in case of error
					cb(CallbackPostType, nr, pid, 0, regs, nil)
				default:
					if ret := t.ReadRet(regs); ret != -int64(syscall.ENOSYS) {
						cb(CallbackPostType, nr, pid, 0, regs, nil)
					}
				}
			}

			if err := syscall.PtraceCont(pid, 0); err != nil {
				t.opts.Logger.Debugf("unable to call ptrace continue for pid %d: %v", pid, err)
			}
		}
	}

	return nil
}

func traceFilterProg(opts TracerOpts) (*syscall.SockFprog, error) {
	policy := seccomp.Policy{
		DefaultAction: seccomp.ActionAllow,
		Syscalls: []seccomp.SyscallGroup{
			{
				Action: seccomp.ActionTrace,
				Names:  opts.Syscalls,
			},
		},
	}

	insts, err := policy.Assemble()
	if err != nil {
		return nil, err
	}
	rawInsts, err := bpf.Assemble(insts)
	if err != nil {
		return nil, err
	}

	filter := make([]syscall.SockFilter, 0, len(rawInsts))
	for _, instruction := range rawInsts {
		filter = append(filter, syscall.SockFilter{
			Code: instruction.Op,
			Jt:   instruction.Jt,
			Jf:   instruction.Jf,
			K:    instruction.K,
		})
	}
	return &syscall.SockFprog{
		Len:    uint16(len(filter)),
		Filter: &filter[0],
	}, nil
}

// NewTracer returns a tracer
func NewTracer(path string, args []string, envs []string, opts TracerOpts) (*Tracer, error) {
	info, err := arch.GetInfo("")
	if err != nil {
		return nil, err
	}

	prog, err := traceFilterProg(opts)
	if err != nil {
		return nil, fmt.Errorf("unable to compile bpf prog: %w", err)
	}

	runtime.LockOSThread()

	pid, err := forkExec(path, args, envs, opts.Creds, prog)
	if err != nil {
		return nil, fmt.Errorf("unable to execute `%s`: %w", path, err)
	}

	var wstatus syscall.WaitStatus
	if _, err = syscall.Wait4(pid, &wstatus, 0, nil); err != nil {
		return nil, fmt.Errorf("unable to call wait4 on `%s`: %w", path, err)
	}

	err = syscall.PtraceSetOptions(pid, ptraceFlags)
	if err != nil {
		return nil, fmt.Errorf("unable to ptrace `%s`, please verify the capabilities: %w", path, err)
	}

	return &Tracer{
		PID:                      pid,
		info:                     info,
		opts:                     opts,
		userCacheRefreshLimiter:  rate.NewLimiter(rate.Every(defaultUserGroupRateLimit), 1),
		groupCacheRefreshLimiter: rate.NewLimiter(rate.Every(defaultUserGroupRateLimit), 1),
	}, nil
}
