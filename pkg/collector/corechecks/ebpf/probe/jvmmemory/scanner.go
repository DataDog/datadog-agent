// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxInspections is the number of consecutive scans at which the memory
	// mappings of a process are inspected looking for libjvm.so. The java
	// launcher loads libjvm.so right after exec, so a process that does not
	// map it at two consecutive scans is not a JVM, until it execs another
	// program (detected by a change of its comm).
	maxInspections = 2

	libjvmName = "libjvm.so"

	// commBufSize is larger than the comm files of procfs (at most 64 bytes).
	commBufSize = 128

	// mapsBufSize is the initial size of the line buffer of findLibJVM,
	// larger than most lines of the maps files.
	mapsBufSize = 4096
	// maxMapsLineLen caps the length of a line of a maps file. The pathname
	// of a mapping is usually at most PATH_MAX (4096) bytes long, up to 4
	// times longer once the kernel escapes its newlines (as "\012"): with
	// the other fields, such a line fits well within the cap.
	maxMapsLineLen = 1 << 20
)

// errRetryLater marks the transient activation errors after which the
// registration of a JVM is retried at the next scan. It always comes with
// utils.ErrEnvironment, which keeps the file out of the block list of the
// file registry: wrap the errors with retryLater.
var errRetryLater = errors.New("retry later")

// retryLater wraps err with errRetryLater and utils.ErrEnvironment.
func retryLater(err error) error {
	return fmt.Errorf("%w: %w: %w", err, errRetryLater, utils.ErrEnvironment)
}

// fileRegistry is the subset of utils.FileRegistry used by processScanner.
type fileRegistry interface {
	Register(namespacedPath string, pid uint32, activationCB, deactivationCB, alreadyRegistered utils.Callback) error
	Unregister(pid uint32) error
}

// pidState is the discovery state of a process.
type pidState struct {
	// inspections is the number of inspections of the memory mappings since
	// the process was first seen or since its comm last changed. inspect
	// resets it to 0 after a transient registration failure, to retry at the
	// next scan, and sets it to maxInspections after a permanent one, to give
	// up until the process execs another program.
	inspections int
	// comm is the comm of the process at its last inspection.
	comm string
	// registered is true when the process is registered in the file
	// registry: it maps a libjvm.so whose probes are attached.
	registered bool
	// startTime is the start time of a registered process, to detect the
	// reuse of its PID by another process between two scans.
	startTime uint64
}

// processScanner discovers the JVMs by polling procfs: the processes mapping
// a libjvm.so are registered in the file registry, which attaches the probes
// once per libjvm.so file and detaches them when the last process using it
// exits.
//
// processScanner is not safe for concurrent use.
type processScanner struct {
	procRoot   string
	registry   fileRegistry
	activate   utils.Callback
	deactivate utils.Callback
	pids       map[uint32]*pidState
	// commBuf is the buffer of readComm.
	commBuf [commBufSize]byte
	// mapsBuf is the initial line buffer of findLibJVM.
	mapsBuf [mapsBufSize]byte
}

func newProcessScanner(procRoot string, registry fileRegistry, activate, deactivate utils.Callback) *processScanner {
	return &processScanner{
		procRoot:   procRoot,
		registry:   registry,
		activate:   activate,
		deactivate: deactivate,
		pids:       make(map[uint32]*pidState),
	}
}

// scanResult is the outcome of a procfs scan.
type scanResult struct {
	// alive is the set of the PIDs found in procfs.
	alive map[uint32]struct{}
	// reused is the set of the alive PIDs of registered JVMs which exited
	// since the previous scan, their PID being reused by another process.
	reused map[uint32]struct{}
	// jvms is the number of JVMs with attached probes.
	jvms int
}

// scan lists the processes of procfs, inspects the new ones, and forgets the
// exited ones.
func (s *processScanner) scan() (scanResult, error) {
	pids, err := kernel.AllPidsProcs(s.procRoot)
	if err != nil {
		return scanResult{}, err
	}

	res := scanResult{
		alive:  make(map[uint32]struct{}, len(pids)),
		reused: make(map[uint32]struct{}),
	}
	for _, p := range pids {
		pid := uint32(p)
		res.alive[pid] = struct{}{}
		if st, ok := s.pids[pid]; ok && st.registered && !s.sameProcess(pid, st) {
			s.forget(pid, st)
			res.reused[pid] = struct{}{}
		}
		s.visit(pid)
	}

	for pid, st := range s.pids {
		if _, ok := res.alive[pid]; ok {
			if st.registered {
				res.jvms++
			}
			continue
		}
		s.forget(pid, st)
	}
	return res, nil
}

// sameProcess returns whether pid still identifies the process registered
// with st, and not another process which reused its PID.
func (s *processScanner) sameProcess(pid uint32, st *pidState) bool {
	startTime, err := readStartTime(s.procRoot, pid)
	// on error, the process most likely exited: the next scan will tell
	return err != nil || startTime == st.startTime
}

// forget unregisters pid if it was registered, and drops its state.
func (s *processScanner) forget(pid uint32, st *pidState) {
	if st.registered {
		if err := s.registry.Unregister(pid); err != nil {
			log.Debugf("jvm_memory: unregistering pid %d: %s", pid, err)
		}
	}
	delete(s.pids, pid)
}

// visit inspects pid if it has not been classified yet.
func (s *processScanner) visit(pid uint32) {
	st, ok := s.pids[pid]
	if !ok {
		st = &pidState{}
		s.pids[pid] = st
	}
	if st.registered {
		return
	}
	comm, commErr := s.readComm(pid)
	if st.inspections >= maxInspections {
		// Not a JVM, unless the process exec'd another program since its
		// last inspection, e.g. an entrypoint script doing `exec java`.
		if commErr != nil || string(comm) == st.comm {
			return
		}
		st.inspections = 0
	}
	if commErr == nil && string(comm) != st.comm {
		st.comm = string(comm)
	}
	s.inspect(pid, st)
}

// inspect looks for a libjvm.so in the memory mappings of pid, and registers
// it in the file registry if found.
func (s *processScanner) inspect(pid uint32, st *pidState) {
	st.inspections++

	path, err := s.findLibJVM(pid)
	if err != nil {
		// most likely the process exited
		log.Tracef("jvm_memory: reading the memory mappings of pid %d: %s", pid, err)
		return
	}
	if path == "" {
		return
	}

	startTime, err := readStartTime(s.procRoot, pid)
	if err != nil {
		// most likely the process exited
		log.Tracef("jvm_memory: reading the start time of pid %d: %s", pid, err)
		return
	}

	err = s.registry.Register(path, pid, s.activate, s.deactivate, utils.IgnoreCB)
	switch {
	case err == nil || errors.Is(err, utils.ErrPathIsAlreadyRegistered):
		st.registered = true
		st.startTime = startTime
	case errors.Is(err, errRetryLater):
		// Transient failure (e.g. too many attached files, or the file
		// changed while being inspected): retry at the next scan.
		st.inspections = 0
		log.Debugf("jvm_memory: not monitoring the JVM of pid %d (%s) yet: %s", pid, path, err)
	default:
		// Do not inspect this JVM again, unless it exec's another program.
		st.inspections = maxInspections
		log.Debugf("jvm_memory: not monitoring the JVM of pid %d (%s): %s", pid, path, err)
	}
}

// readStartTime returns the start time of pid, in clock ticks after boot
// (field 22 of /proc/<pid>/stat).
func readStartTime(procRoot string, pid uint32) (uint64, error) {
	stat, err := os.ReadFile(procPidPath(procRoot, pid, "stat"))
	if err != nil {
		return 0, err
	}
	// The comm (field 2) is between parentheses and may contain spaces and
	// parentheses: the fields are counted from the last closing parenthesis.
	end := bytes.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, fmt.Errorf("malformed stat file of pid %d", pid)
	}
	fields := strings.Fields(string(stat[end+1:]))
	const startTimeIndex = 22 - 3 // fields[0] is field 3
	if len(fields) <= startTimeIndex {
		return 0, fmt.Errorf("malformed stat file of pid %d", pid)
	}
	return strconv.ParseUint(fields[startTimeIndex], 10, 64)
}

// readComm returns the comm of pid. The returned slice is only valid until the
// next call: it is read into the buffer of the scanner, without allocating.
func (s *processScanner) readComm(pid uint32) ([]byte, error) {
	fd, err := unix.Open(procPidPath(s.procRoot, pid, "comm"), unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	n, err := unix.Read(fd, s.commBuf[:])
	unix.Close(fd)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(s.commBuf[:n], []byte{'\n'}), nil
}

// findLibJVM returns the path of the libjvm.so mapped by pid, in the mount
// namespace of pid, or "" if pid does not map any.
func (s *processScanner) findLibJVM(pid uint32) (string, error) {
	f, err := os.Open(procPidPath(s.procRoot, pid, "maps"))
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(s.mapsBuf[:0], maxMapsLineLen)
	libjvm := []byte(libjvmName)
	for scanner.Scan() {
		line := scanner.Bytes()
		// only the lines mentioning libjvm.so are converted to a string
		if !bytes.Contains(line, libjvm) {
			continue
		}
		if path, ok := parseLibJVMMapping(string(line)); ok {
			return path, nil
		}
	}
	return "", scanner.Err()
}

// procPidPath returns the path of /proc/<pid>/<elem...> in the procfs mounted
// at procRoot.
func procPidPath(procRoot string, pid uint32, elem ...string) string {
	return filepath.Join(append([]string{procRoot, strconv.FormatUint(uint64(pid), 10)}, elem...)...)
}

// parseLibJVMMapping returns the path of the file mapped by a line of
// /proc/<pid>/maps, if that file is a libjvm.so.
//
// The format of a line is "address perms offset dev inode pathname", where
// pathname may contain spaces, and ends with " (deleted)" for a deleted file.
func parseLibJVMMapping(line string) (string, bool) {
	var inode string
	for i := 0; i < 5; i++ {
		line = strings.TrimLeft(line, " ")
		end := strings.IndexByte(line, ' ')
		if end < 0 {
			// no pathname
			return "", false
		}
		if i == 4 {
			inode = line[:end]
		}
		line = line[end:]
	}
	path := strings.TrimLeft(line, " ")

	// Anonymous and pseudo mappings have inode 0, deleted files cannot be
	// opened by path anymore.
	if inode == "0" || !strings.HasPrefix(path, "/") || strings.HasSuffix(path, " (deleted)") {
		return "", false
	}
	if filepath.Base(path) != libjvmName {
		return "", false
	}
	return path, true
}
