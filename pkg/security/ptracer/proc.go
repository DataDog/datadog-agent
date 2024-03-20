// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func isKThread(ppid, pid int32) bool {
	return ppid == 2 || pid == 2
}

// ProcProcess defines a process from procfs
type ProcProcess struct {
	*process.Process
	CreateTime int64
}

func collectProcesses(traceePID int32, cache map[int32]int64) ([]*ProcProcess, []int32, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, nil, err
	}

	processes := make(map[int32]*ProcProcess)

	for _, pid := range pids {
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue
		}

		createTime, err := proc.CreateTime()
		if err != nil {
			continue
		}

		processes[pid] = &ProcProcess{
			Process:    proc,
			CreateTime: createTime,
		}
	}

	toIgnore := func(proc *ProcProcess) bool {
		var deja []int32

		// loop to check if proc is not a child of the tracee
		for {
			// protection against infinite loop
			if slices.Contains(deja, proc.Pid) {
				return true
			}
			deja = append(deja, proc.Pid)

			if proc.Pid == traceePID {
				return true
			}

			ppid, err := proc.Ppid()
			if err != nil {
				// if we can get the ppid ignore the process as it can be a race
				return true
			}

			if ppid == 1 || ppid == 0 {
				return false
			}

			if isKThread(ppid, proc.Pid) {
				return true
			}

			// loop with the parent
			if proc = processes[ppid]; proc == nil {
				return true
			}
		}
	}

	var (
		add []*ProcProcess
		del []int32
	)

	for pid, proc := range processes {
		if !toIgnore(proc) && cache[pid] != proc.CreateTime {
			add = append(add, proc)
		}
	}

	for pid := range cache {
		if _, exists := processes[pid]; !exists {
			del = append(del, pid)
		}
	}

	// sort to ensure to add them in the right order
	sort.Slice(add, func(i, j int) bool {
		procA, procB := add[i], add[j]

		if procA.CreateTime == procB.CreateTime {
			return add[i].Pid < add[j].Pid
		}

		return procA.CreateTime < procB.CreateTime
	})

	return add, del, nil
}

func zeroSplitter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == '\x00' {
			return i + 1, data[:i], nil
		}
	}
	if !atEOF {
		return 0, nil, nil
	}
	return 0, data, bufio.ErrFinalToken
}

// TextScannerIterator defines a text scanner iterator
type TextScannerIterator struct {
	file    *os.File
	scanner *bufio.Scanner
}

// NewTextScannerIterator returns a new text scanner iterator
func NewTextScannerIterator(file *os.File) *TextScannerIterator {
	scanner := bufio.NewScanner(file)
	scanner.Split(zeroSplitter)

	return &TextScannerIterator{
		file:    file,
		scanner: scanner,
	}
}

// Reset the iterator
func (t *TextScannerIterator) Reset() {
	scanner := bufio.NewScanner(t.file)
	scanner.Split(zeroSplitter)

	t.scanner = scanner
}

// Next returns true if there is a next element
func (t *TextScannerIterator) Next() bool {
	return t.scanner.Scan()
}

// Text returns the current element
func (t *TextScannerIterator) Text() string {
	return t.scanner.Text()
}

func matchesOnePrefix(text string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func collectPIDEnvVars(pid int32) ([]string, bool, error) {
	filename := fmt.Sprintf("/proc/%d/environ", pid)

	f, err := os.Open(filename)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	it := NewTextScannerIterator(f)
	envs, truncated := truncateEnvs(it)
	return envs, truncated, nil
}

func procToMsg(proc *ProcProcess) (*ebpfless.Message, error) {
	ppid, err := proc.Ppid()
	if err != nil {
		return nil, err
	}

	uids, err := proc.Uids()
	if err != nil {
		return nil, err
	}

	gids, err := proc.Gids()
	if err != nil {
		return nil, err
	}

	cmdLine, err := proc.CmdlineSlice()
	if err != nil {
		return nil, err
	}

	filename, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", proc.Pid))
	if err != nil {
		return nil, fmt.Errorf("snapshot failed for %d: couldn't readlink binary: %w", proc.Pid, err)
	}
	if filename == "/ (deleted)" {
		return nil, fmt.Errorf("snapshot failed for %d: binary was deleted", proc.Pid)
	}

	envs, truncated, _ := collectPIDEnvVars(proc.Pid)

	return &ebpfless.Message{
		Type: ebpfless.MessageTypeSyscall,
		Syscall: &ebpfless.SyscallMsg{
			Type:      ebpfless.SyscallTypeExec,
			PID:       uint32(proc.Pid),
			Timestamp: uint64(time.Unix(0, proc.CreateTime*int64(time.Millisecond)).UnixNano()),
			Exec: &ebpfless.ExecSyscallMsg{
				File: ebpfless.FileSyscallMsg{
					Filename: filename,
				},
				Args:          cmdLine,
				Envs:          envs,
				EnvsTruncated: truncated,
				TTY:           getPidTTY(int(proc.Pid)),
				Credentials: &ebpfless.Credentials{
					UID:  uint32(uids[0]),
					EUID: uint32(uids[1]),
					GID:  uint32(gids[0]),
					EGID: uint32(gids[1]),
				},
				PPID: uint32(ppid),
			},
		},
	}, nil
}

func scanProcfs(ctx context.Context, traceePID int, sendFnc func(msg *ebpfless.Message), every time.Duration, logger Logger) {
	cache := make(map[int32]int64)

	ticker := time.NewTicker(every)

	for {
		select {
		case <-ticker.C:
			add, del, err := collectProcesses(int32(traceePID), cache)
			if err != nil {
				logger.Errorf("unable to collect processes: %v", err)
				continue
			}

			for _, proc := range add {
				if msg, err := procToMsg(proc); err == nil {
					sendFnc(msg)
				}
				cache[proc.Pid] = proc.CreateTime
			}

			// cleanup
			for _, pid := range del {
				delete(cache, pid)

				msg := &ebpfless.Message{
					Type: ebpfless.MessageTypeSyscall,
					Syscall: &ebpfless.SyscallMsg{
						Type:      ebpfless.SyscallTypeExit,
						PID:       uint32(pid),
						Timestamp: uint64(time.Now().UnixNano()),
						Exit:      &ebpfless.ExitSyscallMsg{},
					},
				}
				sendFnc(msg)
			}
		case <-ctx.Done():
			return
		}
	}
}
