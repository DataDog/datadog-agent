// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// seccomp_data field offsets (see linux/seccomp.h)
	offsetNr   = 0
	offsetArch = 4

	// seccomp_data total size: nr(4) + arch(4) + instruction_pointer(8) + args[6](48) = 64
	seccompDataSize = 64

	// SECCOMP_RET action constants (top 16 bits)
	seccompRetActionMask     = 0xFFFF0000
	seccompRetKillProcess    = 0x80000000
	seccompRetKillThread     = 0x00000000
	seccompRetTrap           = 0x00030000
	seccompRetErrno          = 0x00050000
	seccompRetUserNotif      = 0x7FC00000
	seccompRetTrace          = 0x7FF00000
	seccompRetLog            = 0x7FFC0000
	seccompRetAllow          = 0x7FFF0000

	// AUDIT_ARCH values
	auditArchX86_64  = 0xC000003E
	auditArchAArch64 = 0xC00000B7

	// Max syscall NR to probe
	maxSyscallNR = 512
)

// SeccompAction is the effective action for a syscall.
type SeccompAction string

const (
	ActionAllow       SeccompAction = "ALLOW"
	ActionErrno       SeccompAction = "ERRNO"
	ActionKillThread  SeccompAction = "KILL_THREAD"
	ActionKillProcess SeccompAction = "KILL_PROCESS"
	ActionTrap        SeccompAction = "TRAP"
	ActionTrace       SeccompAction = "TRACE"
	ActionLog         SeccompAction = "LOG"
	ActionNotify      SeccompAction = "USER_NOTIF"
)

// ArgCondition describes one argument-level check extracted from a BPF filter.
type ArgCondition struct {
	Index  int           // args[0..5]
	Op     string        // "==", "!=", "&"
	Value  uint32        // constant compared against
	Action SeccompAction // action when condition matches
}

// SyscallRule is the effective action for a syscall, optionally with argument conditions.
type SyscallRule struct {
	Action        SeccompAction  // base action (evaluated with args=0)
	ArgConditions []ArgCondition // extracted argument-level conditions, may be nil
}

// SeccompFilterResult holds the full result of extracting a seccomp filter.
type SeccompFilterResult struct {
	DefaultAction SeccompAction
	Syscalls      map[string]SyscallRule // syscall name → rule with arg conditions
}

// ExtractSeccompFilter attaches to pid via ptrace, reads all seccomp BPF
// filters, emulates them for every syscall NR, and returns the allowed list.
func ExtractSeccompFilter(pid int, arch string) (*SeccompFilterResult, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := syscall.PtraceAttach(pid); err != nil {
		return nil, fmt.Errorf("ptrace attach pid %d: %w", pid, err)
	}
	defer func() { _ = syscall.PtraceDetach(pid) }()

	var ws syscall.WaitStatus
	if _, err := syscall.Wait4(pid, &ws, 0, nil); err != nil {
		return nil, fmt.Errorf("wait4 pid %d: %w", pid, err)
	}

	filters, err := readSeccompFilters(pid)
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return nil, nil // no seccomp filter installed
	}

	auditArch := archToAudit(arch)
	actions := emulateFilters(filters, auditArch)

	result := &SeccompFilterResult{
		Syscalls: make(map[string]SyscallRule, len(actions)),
	}

	result.DefaultAction = verdictToAction(evalFilters(filters, auditArch, maxSyscallNR+1))

	// Static analysis: extract argument conditions from each filter.
	argCondsByNR := make(map[int][]ArgCondition)
	for _, insts := range filters {
		for nr, conds := range analyzeArgConditions(insts) {
			argCondsByNR[nr] = append(argCondsByNR[nr], conds...)
		}
	}

	syscallArch := normalizeArch(arch)
	for nr, action := range actions {
		name, ok := utils.Syscalls[utils.SyscallKey{Arch: syscallArch, ID: nr}]
		if !ok {
			continue
		}
		rule := SyscallRule{Action: action}
		if conds, ok := argCondsByNR[nr]; ok {
			rule.ArgConditions = conds
		}
		result.Syscalls[name] = rule
	}
	return result, nil
}

// readSeccompFilters reads all stacked seccomp BPF filters from a ptraced process.
func readSeccompFilters(pid int) ([][]bpf.Instruction, error) {
	var filters [][]bpf.Instruction

	for idx := 0; ; idx++ {
		n, err := ptraceSeccompGetFilter(pid, idx, nil)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				break // no more filters
			}
			if errors.Is(err, unix.EACCES) {
				return nil, fmt.Errorf("PTRACE_SECCOMP_GET_FILTER: missing CAP_SYS_ADMIN")
			}
			return nil, fmt.Errorf("PTRACE_SECCOMP_GET_FILTER size (index %d): %w", idx, err)
		}
		if n == 0 {
			continue
		}

		raw := make([]rawSockFilter, n)
		if _, err := ptraceSeccompGetFilter(pid, idx, raw); err != nil {
			return nil, fmt.Errorf("PTRACE_SECCOMP_GET_FILTER data (index %d): %w", idx, err)
		}

		rawInsts := make([]bpf.RawInstruction, n)
		for i, sf := range raw {
			rawInsts[i] = bpf.RawInstruction{Op: sf.Code, Jt: sf.Jt, Jf: sf.Jf, K: sf.K}
		}
		insts, _ := bpf.Disassemble(rawInsts)
		filters = append(filters, insts)
	}
	return filters, nil
}

type rawSockFilter struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// ptraceSeccompGetFilter wraps ptrace(PTRACE_SECCOMP_GET_FILTER, pid, index, buf).
// If buf is nil, returns the number of instructions.
func ptraceSeccompGetFilter(pid, index int, buf []rawSockFilter) (int, error) {
	var dataPtr unsafe.Pointer
	if buf != nil {
		dataPtr = unsafe.Pointer(&buf[0])
	}
	n, _, errno := syscall.RawSyscall6(
		syscall.SYS_PTRACE,
		uintptr(unix.PTRACE_SECCOMP_GET_FILTER),
		uintptr(pid),
		uintptr(index),
		uintptr(dataPtr),
		0, 0,
	)
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}

// emulateFilters runs every syscall NR through all stacked filters and returns
// the effective action for each. Seccomp uses most-restrictive-wins across
// the filter stack.
func emulateFilters(filters [][]bpf.Instruction, auditArch uint32) map[int]SeccompAction {
	vms := make([]*bpf.VM, 0, len(filters))
	for _, insts := range filters {
		vm, err := bpf.NewVM(insts)
		if err != nil {
			continue
		}
		vms = append(vms, vm)
	}
	if len(vms) == 0 {
		return nil
	}

	actions := make(map[int]SeccompAction, maxSyscallNR)
	for nr := 0; nr < maxSyscallNR; nr++ {
		actions[nr] = verdictToAction(runFilters(vms, nr, auditArch))
	}
	return actions
}

// evalFilters builds VMs and evaluates a single syscall NR across all filters.
func evalFilters(filters [][]bpf.Instruction, auditArch uint32, nr int) uint32 {
	vms := make([]*bpf.VM, 0, len(filters))
	for _, insts := range filters {
		vm, err := bpf.NewVM(insts)
		if err != nil {
			continue
		}
		vms = append(vms, vm)
	}
	return runFilters(vms, nr, auditArch)
}

// runFilters evaluates a syscall NR against all VMs and returns the
// most-restrictive verdict (lowest precedence wins).
func runFilters(vms []*bpf.VM, nr int, auditArch uint32) uint32 {
	data := buildSeccompData(nr, auditArch)
	var mostRestrictive uint32 = seccompRetAllow
	for _, vm := range vms {
		verdict, err := vm.Run(data)
		if err != nil {
			return seccompRetKillThread
		}
		v := uint32(verdict) & seccompRetActionMask
		if v < mostRestrictive {
			mostRestrictive = v
		}
	}
	return mostRestrictive
}

func verdictToAction(v uint32) SeccompAction {
	switch v {
	case seccompRetAllow:
		return ActionAllow
	case seccompRetLog:
		return ActionLog
	case seccompRetTrace:
		return ActionTrace
	case seccompRetUserNotif:
		return ActionNotify
	case seccompRetErrno:
		return ActionErrno
	case seccompRetTrap:
		return ActionTrap
	case seccompRetKillProcess:
		return ActionKillProcess
	default:
		return ActionKillThread
	}
}

func buildSeccompData(nr int, auditArch uint32) []byte {
	data := make([]byte, seccompDataSize)
	binary.NativeEndian.PutUint32(data[offsetNr:], uint32(nr))
	binary.NativeEndian.PutUint32(data[offsetArch:], auditArch)
	return data
}

func archToAudit(arch string) uint32 {
	switch arch {
	case "arm64":
		return auditArchAArch64
	default: // "amd64", "x64", ""
		return auditArchX86_64
	}
}

// normalizeArch maps runtime arch strings to the keys used in utils.Syscalls.
func normalizeArch(arch string) string {
	switch arch {
	case "x64":
		return "amd64"
	case "":
		return "amd64"
	default:
		return arch
	}
}

// offsetArgs is the byte offset of seccomp_data.args[0] in the seccomp_data struct.
const offsetArgs = 16

// analyzeArgConditions walks a single BPF filter's instructions and extracts
// argument-level conditions for each syscall NR. It returns a map from syscall
// NR to the conditions found.
//
// The analysis tracks three pieces of state as it walks:
//   - currentNR: the syscall NR established by the most recent JumpIf on offset 0
//   - argIndex:  which args[N] was last loaded (from LoadAbsolute at offset 16..63)
//   - masked:    whether an ALUOpAnd was applied (bitmask check)
//   - maskVal:   the AND mask constant
func analyzeArgConditions(insts []bpf.Instruction) map[int][]ArgCondition {
	result := make(map[int][]ArgCondition)

	type state struct {
		currentNR   int  // -1 = unknown
		argIndex    int  // -1 = not an arg load
		lastLoadOff int  // offset of last LoadAbsolute
		masked      bool // ALUOpAnd was applied after arg load
		maskVal     uint32
	}
	s := state{currentNR: -1, argIndex: -1, lastLoadOff: -1}

	for i, inst := range insts {
		switch v := inst.(type) {
		case bpf.LoadAbsolute:
			s.masked = false
			s.lastLoadOff = int(v.Off)
			if v.Off == offsetNr {
				s.argIndex = -1
			} else if v.Off >= offsetArgs && v.Off < offsetArgs+48 {
				s.argIndex = int(v.Off-offsetArgs) / 8
			} else {
				s.argIndex = -1
			}

		case bpf.ALUOpConstant:
			if v.Op == bpf.ALUOpAnd && s.argIndex >= 0 {
				s.masked = true
				s.maskVal = v.Val
			}

		case bpf.JumpIf:
			if s.lastLoadOff == offsetNr && v.Cond == bpf.JumpEqual {
				// This is a syscall NR check — update the tracked NR.
				s.currentNR = int(v.Val)
				s.argIndex = -1
				continue
			}
			if s.argIndex >= 0 && s.currentNR >= 0 {
				conds := extractConditions(v, s.argIndex, s.masked, s.maskVal, insts, i)
				result[s.currentNR] = append(result[s.currentNR], conds...)
			}

		case bpf.RetConstant:
			// A return resets arg tracking but not the current NR context.
			s.argIndex = -1
			s.masked = false
		}
	}
	return result
}

// extractConditions produces ArgConditions from a JumpIf on an arg load.
func extractConditions(j bpf.JumpIf, argIdx int, masked bool, maskVal uint32, insts []bpf.Instruction, pc int) []ArgCondition {
	var conds []ArgCondition

	trueAction := resolveTarget(insts, pc+1+int(j.SkipTrue))
	falseAction := resolveTarget(insts, pc+1+int(j.SkipFalse))

	if masked {
		// Pattern: A = args[N]; A &= MASK; if A <cond> val
		// Most common: A &= MASK; if A != 0 → bitmask check
		switch j.Cond {
		case bpf.JumpBitsSet:
			if trueAction != "" {
				conds = append(conds, ArgCondition{Index: argIdx, Op: "&", Value: j.Val, Action: trueAction})
			}
		case bpf.JumpBitsNotSet:
			if falseAction != "" {
				conds = append(conds, ArgCondition{Index: argIdx, Op: "&", Value: j.Val, Action: falseAction})
			}
		default:
			// ALUOpAnd + JumpEqual/NotEqual on 0 is the typical pattern
			if j.Cond == bpf.JumpEqual && j.Val == 0 {
				// A & MASK == 0: false branch means bits are set
				if falseAction != "" {
					conds = append(conds, ArgCondition{Index: argIdx, Op: "&", Value: maskVal, Action: falseAction})
				}
			} else if j.Cond == bpf.JumpNotEqual && j.Val == 0 {
				if trueAction != "" {
					conds = append(conds, ArgCondition{Index: argIdx, Op: "&", Value: maskVal, Action: trueAction})
				}
			}
		}
		return conds
	}

	switch j.Cond {
	case bpf.JumpEqual:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: "==", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpNotEqual:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: "!=", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpGreaterThan:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: ">", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpGreaterOrEqual:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: ">=", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpLessThan:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: "<", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpLessOrEqual:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: "<=", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpBitsSet:
		if trueAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: "&", Value: j.Val, Action: trueAction})
		}
	case bpf.JumpBitsNotSet:
		if falseAction != "" {
			conds = append(conds, ArgCondition{Index: argIdx, Op: "&", Value: j.Val, Action: falseAction})
		}
	}
	return conds
}

// resolveTarget looks at the instruction at the given index and returns
// the SeccompAction if it's a RetConstant, or "" if it's not directly resolvable.
func resolveTarget(insts []bpf.Instruction, idx int) SeccompAction {
	if idx < 0 || idx >= len(insts) {
		return ""
	}
	if ret, ok := insts[idx].(bpf.RetConstant); ok {
		return verdictToAction(ret.Val & seccompRetActionMask)
	}
	return ""
}

