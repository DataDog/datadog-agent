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
	"os"
	"os/signal"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
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
	syscallHandlers map[int]syscallHandler
	PtracedSyscalls []string

	// PID represents a PID
	pidLock sync.RWMutex
	PIDs    []int
	entry   string
	Args    []string
	Envs    []string

	// internals
	info *arch.Info
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
	pageSize := uint64(os.Getpagesize())
	pageAddr := ptr & ^(pageSize - 1)
	sizeToEndOfPage := pageAddr + pageSize - ptr
	// read from at most 2 pages (current and next one)
	maxReadSize := sizeToEndOfPage + pageSize

	// start by reading from the current page
	for readSize := sizeToEndOfPage; readSize <= maxReadSize; readSize += pageSize {
		data := make([]byte, readSize)
		_, err := processVMReadv(pid, uintptr(ptr), data)
		if err != nil {
			return "", fmt.Errorf("unable to read string at addr %x (size: %d): %v", ptr, readSize, err)
		}

		n := bytes.Index(data[:], []byte{0})
		if n >= 0 {
			return string(data[:n]), nil
		}
	}

	return "", fmt.Errorf("unable to read string at addr %x: string is too long", ptr)
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
	err = binary.Read(buf, binary.NativeEndian, &val)
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

		ptr := binary.NativeEndian.Uint64(data)
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

func isExited(waitStatus syscall.WaitStatus) bool {
	return waitStatus.Exited() || waitStatus.CoreDump() || waitStatus.Signaled()
}

func (t *Tracer) pidExited(pid int) int {
	t.pidLock.Lock()
	defer t.pidLock.Unlock()

	t.PIDs = slices.DeleteFunc(t.PIDs, func(p int) bool {
		return p == pid
	})
	return len(t.PIDs)
}

func (ctx *CWSPtracerCtx) trace() error {
	var waitStatus syscall.WaitStatus

	ctx.pidLock.RLock()
	for _, pid := range ctx.PIDs {
		if err := syscall.PtraceSyscall(pid, 0); err != nil {
			ctx.pidLock.RUnlock()
			return err
		}
	}
	ctx.pidLock.RUnlock()

	var (
		tracker = NewSyscallStateTracker()
		regs    syscall.PtraceRegs
	)

	for {
		pid, err := syscall.Wait4(-1, &waitStatus, 0, nil)
		if err != nil {
			logger.Errorf("unable to wait for pid %d: %v", pid, err)
			break
		}

		if isExited(waitStatus) {
			tracker.Exit(pid)

			if ctx.pidExited(pid) == 0 {
				break
			}
			ctx.handleHooks(CallbackExitType, ExitNr, pid, 0, regs, &waitStatus)
			continue
		}

		if waitStatus.Stopped() {
			if signal := waitStatus.StopSignal(); signal != syscall.SIGTRAP {
				if signal == syscall.SIGSTOP {
					signal = syscall.Signal(0)
				}
				if err := syscall.PtraceSyscall(pid, int(signal)); err == nil {
					continue
				}
			}

			if err := syscall.PtraceGetRegs(pid, &regs); err != nil {
				logger.Logf("unable to get registers for pid %d: %v", pid, err)

				// it got probably killed
				ctx.handleHooks(CallbackExitType, ExitNr, pid, 0, regs, &waitStatus)

				if ctx.pidExited(pid) == 0 {
					break
				}

				continue
			}

			nr := GetSyscallNr(regs)

			switch waitStatus.TrapCause() {
			case syscall.PTRACE_EVENT_CLONE, syscall.PTRACE_EVENT_FORK, syscall.PTRACE_EVENT_VFORK:
				// called at the exit of the syscall
				if npid, err := syscall.PtraceGetEventMsg(pid); err == nil {
					ctx.handleHooks(CallbackPostType, nr, int(npid), pid, regs, nil)
				}
			case syscall.PTRACE_EVENT_EXEC:
				// called at the exit of the syscall
				ctx.handleHooks(CallbackPostType, ExecveNr, pid, 0, regs, nil)

				if state := tracker.PeekState(pid); state != nil {
					state.Exec = true
				}
			default:
				switch nr {
				case ForkNr, VforkNr, CloneNr, Clone3Nr:
					// already handled by the PTRACE_EVENT_CLONE, etc.
				default:
					state := tracker.NextStop(pid)

					if state.Entry {
						ctx.handleHooks(CallbackPreType, nr, pid, 0, regs, nil)
					} else {
						// we already captured the exit of the exec syscall with PTRACE_EVENT_EXEC if success
						if !state.Exec {
							ctx.handleHooks(CallbackPostType, nr, pid, 0, regs, nil)
						}
						state.Exec = false
					}
				}
			}

			if err := syscall.PtraceSyscall(pid, 0); err != nil {
				logger.Errorf("unable to call ptrace continue for pid %d: %v", pid, err)
			}
		}
	}

	return nil
}

func (ctx *CWSPtracerCtx) traceWithSeccomp() error {
	var waitStatus syscall.WaitStatus

	ctx.pidLock.RLock()
	for _, pid := range ctx.PIDs {
		if err := syscall.PtraceCont(pid, 0); err != nil {
			ctx.pidLock.RUnlock()
			return err
		}
	}
	ctx.pidLock.RUnlock()

	var (
		regs syscall.PtraceRegs
	)

	for {
		pid, err := syscall.Wait4(-1, &waitStatus, 0, nil)
		if err != nil {
			logger.Errorf("unable to wait for pid %d: %v", pid, err)
			break
		}

		if isExited(waitStatus) {
			if ctx.pidExited(pid) == 0 {
				break
			}
			ctx.handleHooks(CallbackExitType, ExitNr, pid, 0, regs, &waitStatus)
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
				logger.Logf("unable to get registers for pid %d: %v", pid, err)

				// it got probably killed
				ctx.handleHooks(CallbackExitType, ExitNr, pid, 0, regs, &waitStatus)

				if ctx.pidExited(pid) == 0 {
					break
				}

				continue
			}

			nr := GetSyscallNr(regs)

			switch waitStatus.TrapCause() {
			case syscall.PTRACE_EVENT_CLONE, syscall.PTRACE_EVENT_FORK, syscall.PTRACE_EVENT_VFORK:
				// called at the exit of the syscall
				if npid, err := syscall.PtraceGetEventMsg(pid); err == nil {
					ctx.handleHooks(CallbackPostType, nr, int(npid), pid, regs, nil)
				}
			case syscall.PTRACE_EVENT_EXEC:
				// called at the exit of the syscall
				ctx.handleHooks(CallbackPostType, ExecveNr, pid, 0, regs, nil)
			case unix.PTRACE_EVENT_SECCOMP:
				switch nr {
				case ForkNr, VforkNr, CloneNr, Clone3Nr:
					// already handled by the PTRACE_EVENT_CLONE, etc.
				default:
					ctx.handleHooks(CallbackPreType, nr, pid, 0, regs, nil)

					// force a ptrace syscall in order to get to return value
					if err := syscall.PtraceSyscall(pid, 0); err != nil {
						logger.Errorf("unable to call ptrace syscall for pid %d: %v", pid, err)
					}
					continue
				}
			default:
				switch nr {
				case ForkNr, VforkNr, CloneNr, Clone3Nr:
					// already handled by the PTRACE_EVENT_CLONE, etc.
				case ExecveNr, ExecveatNr:
					// triggered in case of error
					ctx.handleHooks(CallbackPostType, nr, pid, 0, regs, nil)
				default:
					if ret := ctx.ReadRet(regs); ret != -int64(syscall.ENOSYS) {
						ctx.handleHooks(CallbackPostType, nr, pid, 0, regs, nil)
					}
				}
			}

			if err := syscall.PtraceCont(pid, 0); err != nil {
				logger.Errorf("unable to call ptrace continue for pid %d: %v", pid, err)
			}
		}
	}

	return nil
}

// Trace traces a process
func (ctx *CWSPtracerCtx) Trace() error {
	if ctx.opts.SeccompDisabled {
		return ctx.trace()
	}
	return ctx.traceWithSeccomp()
}

func traceFilterProg(ptracedSyscalls []string) (*syscall.SockFprog, error) {
	policy := seccomp.Policy{
		DefaultAction: seccomp.ActionAllow,
		Syscalls: []seccomp.SyscallGroup{
			{
				Action: seccomp.ActionTrace,
				Names:  ptracedSyscalls,
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

// AttachTracer attach the tracer to the given pid
func (ctx *CWSPtracerCtx) AttachTracer() error {
	info, err := arch.GetInfo("")
	if err != nil {
		return err
	}
	ctx.info = info

	runtime.LockOSThread()

	for _, pid := range ctx.PIDs {
		if err := syscall.PtraceAttach(pid); err != nil {
			return fmt.Errorf("unable to attach to pid `%d`: %w", pid, err)
		}

		var wstatus syscall.WaitStatus
		if _, err := syscall.Wait4(pid, &wstatus, 0, nil); err != nil {
			return fmt.Errorf("unable to call wait4 on `%d`: %w", pid, err)
		}

		err = syscall.PtraceSetOptions(pid, ptraceFlags)
		if err != nil {
			return fmt.Errorf("unable to ptrace `%d`, please verify the capabilities: %w", pid, err)
		}

		// first process
		process := NewProcess(pid)
		ctx.processCache.Add(pid, process)
	}

	ctx.userCacheRefreshLimiter = rate.NewLimiter(rate.Every(defaultUserGroupRateLimit), 1)
	ctx.groupCacheRefreshLimiter = rate.NewLimiter(rate.Every(defaultUserGroupRateLimit), 1)
	return nil
}

// NewTracer returns a tracer
func (ctx *CWSPtracerCtx) NewTracer() error {
	info, err := arch.GetInfo("")
	if err != nil {
		return err
	}
	ctx.info = info

	var prog *syscall.SockFprog

	// syscalls specified then we generate a seccomp filter
	if !ctx.opts.SeccompDisabled {
		prog, err = traceFilterProg(ctx.PtracedSyscalls)
		if err != nil {
			return fmt.Errorf("unable to compile bpf prog: %w", err)
		}
	}

	runtime.LockOSThread()

	pid, err := forkExec(ctx.entry, ctx.Args, ctx.Envs, ctx.opts.Creds, prog)
	if err != nil {
		return fmt.Errorf("unable to execute `%s`: %w", ctx.entry, err)
	}
	ctx.PIDs = []int{pid}

	var wstatus syscall.WaitStatus
	if _, err = syscall.Wait4(pid, &wstatus, 0, nil); err != nil {
		return fmt.Errorf("unable to call wait4 on `%s`: %w", ctx.entry, err)
	}

	err = syscall.PtraceSetOptions(pid, ptraceFlags)
	if err != nil {
		return fmt.Errorf("unable to ptrace `%s`, please verify the capabilities: %w", ctx.entry, err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-sigChan
		os.Exit(0)
	}()

	// first process
	process := NewProcess(pid)
	ctx.processCache.Add(pid, process)

	ctx.userCacheRefreshLimiter = rate.NewLimiter(rate.Every(defaultUserGroupRateLimit), 1)
	ctx.groupCacheRefreshLimiter = rate.NewLimiter(rate.Every(defaultUserGroupRateLimit), 1)
	return nil
}
