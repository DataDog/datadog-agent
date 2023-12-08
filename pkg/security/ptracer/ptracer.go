// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

import (
	"bytes"
	"os"
	"runtime"
	"syscall"

	"github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"

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

	// Nsig number of signal
	// https://elixir.bootlin.com/linux/v6.5.12/source/arch/x86/include/uapi/asm/signal.h#L16
	Nsig = 32
)

// Tracer represents a tracer
type Tracer struct {
	// PID represents a PID
	PID int

	// internals
	info *arch.Info
}

// Opts defines syscall filters
type Opts struct {
	Syscalls []string
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
func (t *Tracer) Trace(cb func(cbType CallbackType, nr int, pid int, ppid int, regs syscall.PtraceRegs)) error {
	var waitStatus syscall.WaitStatus

	if err := syscall.PtraceCont(t.PID, 0); err != nil {
		return err
	}

	var regs syscall.PtraceRegs

	for {
		pid, err := syscall.Wait4(-1, &waitStatus, 0, nil)
		if err != nil {
			break
		}

		if waitStatus.Exited() || waitStatus.Signaled() {
			if pid == t.PID {
				break
			}
			cb(CallbackExitType, ExitNr, pid, 0, regs)
			continue
		}

		if waitStatus.Stopped() {
			if signal := waitStatus.StopSignal(); signal != syscall.SIGTRAP {
				if signal < Nsig {
					_ = syscall.PtraceCont(pid, int(signal))
					continue
				}
			}

			if err := syscall.PtraceGetRegs(pid, &regs); err != nil {
				break
			}

			nr := GetSyscallNr(regs)

			switch waitStatus.TrapCause() {
			case syscall.PTRACE_EVENT_CLONE, syscall.PTRACE_EVENT_FORK, syscall.PTRACE_EVENT_VFORK:
				if npid, err := syscall.PtraceGetEventMsg(pid); err == nil {
					cb(CallbackPostType, nr, int(npid), pid, regs)
				}
			case unix.PTRACE_EVENT_SECCOMP:
				switch nr {
				case ForkNr, VforkNr, CloneNr:
					// already handled
				default:
					cb(CallbackPreType, nr, pid, 0, regs)

					// force a ptrace syscall in order to get to return value
					if err := syscall.PtraceSyscall(pid, 0); err != nil {
						continue
					}
				}
			default:
				switch nr {
				case ForkNr, VforkNr, CloneNr:
					// already handled
				case ExecveNr, ExecveatNr:
					// does not return on success, thus ret value stay at syscall.ENOSYS
					if ret := -t.ReadRet(regs); ret == int64(syscall.ENOSYS) {
						cb(CallbackPostType, nr, pid, 0, regs)
					}
				default:
					if ret := -t.ReadRet(regs); ret != int64(syscall.ENOSYS) {
						cb(CallbackPostType, nr, pid, 0, regs)
					}
				}
			}

			if err := syscall.PtraceCont(pid, 0); err != nil {
				continue
			}
		}
	}

	return nil
}

func traceFilterProg(opts Opts) (*syscall.SockFprog, error) {
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
func NewTracer(path string, args []string, opts Opts) (*Tracer, error) {

	info, err := arch.GetInfo("")
	if err != nil {
		return nil, err
	}

	prog, err := traceFilterProg(opts)
	if err != nil {
		return nil, err
	}

	runtime.LockOSThread()

	pid, err := forkExec(path, args, os.Environ(), prog)
	if err != nil {
		return nil, err
	}

	var wstatus syscall.WaitStatus
	if _, err = syscall.Wait4(pid, &wstatus, 0, nil); err != nil {
		return nil, err
	}

	err = syscall.PtraceSetOptions(pid, ptraceFlags)
	if err != nil {
		return nil, err
	}

	return &Tracer{
		PID:  pid,
		info: info,
	}, nil
}
