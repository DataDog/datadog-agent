// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package darwin

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
)

// Snapshot inserts every currently running process into the resolver so that
// process trees are not truncated at collector startup.
//
// Without it, a tree only reaches back as far as the first fork the collector
// happened to observe: anything already running, including the shell and the
// package manager that started the activity, is missing. That is visible in the
// product as an ancestor row with a pid and nothing else.
//
// No cgo is needed. golang.org/x/sys/unix exposes the process list, ppid,
// credentials and start time through one sysctl; only argv needs manual work.
func Snapshot(pr *process.EBPFLessResolver) (int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return 0, fmt.Errorf("kern.proc.all: %w", err)
	}

	// Ascending pid order means a parent is usually inserted before its children,
	// which is what lets AddProcFSEntry link them. It is not a guarantee -- pids
	// wrap -- so a child whose parent comes later simply stays unlinked rather
	// than linking to the wrong process.
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].Proc.P_pid < procs[j].Proc.P_pid
	})

	var inserted int
	for i := range procs {
		kp := &procs[i]
		pid := uint32(kp.Proc.P_pid)
		if pid == 0 {
			continue
		}

		// A failure here is normal for processes we cannot read, and the entry is
		// still worth inserting for its position in the tree.
		execPath, argv := "", []string(nil)
		if blob, err := unix.SysctlRaw("kern.procargs2", int(pid)); err == nil {
			execPath, argv, _ = parseProcArgs(blob)
		}

		start := time.Unix(kp.Proc.P_starttime.Sec, int64(kp.Proc.P_starttime.Usec)*1000)

		entry := pr.AddProcFSEntry(
			process.CacheResolverKey{Pid: pid},
			uint32(kp.Eproc.Ppid),
			execPath,
			argv,
			false, // argsTruncated
			nil,   // envs: never captured
			false, // envsTruncated
			"",    // container ID: no meaning on a laptop
			"",    // cgroup ID: likewise
			uint64(start.UnixNano()),
			"", // tty: not available from kinfo_proc
		)
		if entry == nil {
			continue
		}

		entry.Credentials.UID = kp.Eproc.Pcred.P_ruid
		entry.Credentials.EUID = kp.Eproc.Ucred.Uid
		entry.Credentials.GID = kp.Eproc.Pcred.P_rgid
		inserted++
	}

	return inserted, nil
}

// parseProcArgs decodes a KERN_PROCARGS2 blob: a 4-byte argc, the executable
// path, a run of NUL padding, then argc NUL-terminated argv strings, and finally
// the environment.
//
// It deliberately stops after argc strings and never reads the environment:
// environment variables are not captured on a developer laptop. Note that this
// is the one place where the environment is sitting right there in the buffer, so
// the argc bound is load-bearing rather than incidental.
func parseProcArgs(blob []byte) (execPath string, argv []string, err error) {
	if len(blob) < 4 {
		return "", nil, errors.New("procargs2 blob too short")
	}
	argc := int(binary.LittleEndian.Uint32(blob[:4]))
	rest := blob[4:]

	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return "", nil, errors.New("no exec path in procargs2")
	}
	execPath = string(rest[:end])

	// Skip the run of NUL padding between the exec path and argv.
	for end < len(rest) && rest[end] == 0 {
		end++
	}
	rest = rest[end:]

	for i := 0; i < argc && len(rest) > 0; i++ {
		n := bytes.IndexByte(rest, 0)
		if n < 0 {
			argv = append(argv, string(rest))
			break
		}
		argv = append(argv, string(rest[:n]))
		rest = rest[n+1:]
	}

	return execPath, argv, nil
}
