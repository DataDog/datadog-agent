// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sysctl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	direntBufferSize       = 8192
	initialValueBufferSize = 4096

	direntReclenOffset = int(unsafe.Offsetof(unix.Dirent{}.Reclen))
	direntTypeOffset   = int(unsafe.Offsetof(unix.Dirent{}.Type))
	direntNameOffset   = int(unsafe.Offsetof(unix.Dirent{}.Name))
)

var redactedValue = []byte(redactedContent)

// WalkProcSys calls fn with the name and the current value of every readable
// system control parameter in /proc/sys. The name is the path relative to
// /proc/sys, as reported by sysctl events (e.g. "kernel/yama/ptrace_scope"),
// and the value has its surrounding whitespace trimmed, as in sysctl events.
// The values of the parameters listed in ignoredBaseNames are redacted, as in
// the snapshot.
//
// A host has thousands of parameters, about 130 more per network interface, so
// the walk reuses its buffers and doesn't allocate per parameter: name and value
// are only valid until fn returns, and fn must copy them to keep them.
func WalkProcSys(ignoredBaseNames []string, fn func(name, value []byte)) error {
	return walkSystemControls(kernel.HostProc("/sys"), ignoredBaseNames, fn)
}

// walkSystemControls calls fn with the name, relative to root, and the trimmed
// value of every readable regular file under root. Directories and files that
// can't be read are skipped, such as fs/binfmt_misc when it is an automount
// point that can't be triggered from a container (ELOOP). Symbolic links aren't
// followed.
func walkSystemControls(root string, ignoredBaseNames []string, fn func(name, value []byte)) error {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: root, Err: err}
	}
	defer unix.Close(fd)

	w := systemControlsWalker{
		ignoredBaseNames: ignoredBaseNames,
		fn:               fn,
		name:             make([]byte, 0, 256),
		value:            make([]byte, initialValueBufferSize),
	}
	if err := w.walkDir(fd, 0); err != nil {
		return &os.PathError{Op: "getdents", Path: root, Err: err}
	}
	return nil
}

type systemControlsWalker struct {
	ignoredBaseNames []string
	fn               func(name, value []byte)

	// name is the path of the current entry, relative to the root
	name []byte
	// dirents holds a getdents buffer per depth, as the walk enters a
	// directory before it is done with the entries of its parent
	dirents [][]byte
	value   []byte
	stat    unix.Stat_t
}

func (w *systemControlsWalker) walkDir(dirfd int, depth int) error {
	if depth == len(w.dirents) {
		w.dirents = append(w.dirents, make([]byte, direntBufferSize))
	}
	buf := w.dirents[depth]

	for {
		n, err := unix.Getdents(dirfd, buf)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if n <= 0 {
			return nil
		}

		for off := 0; off+direntNameOffset < n; {
			reclen := int(binary.NativeEndian.Uint16(buf[off+direntReclenOffset:]))
			if reclen <= direntNameOffset || off+reclen > n {
				return errors.New("malformed directory entry")
			}
			entryType := buf[off+direntTypeOffset]
			nameZ := buf[off+direntNameOffset : off+reclen]
			off += reclen

			end := bytes.IndexByte(nameZ, 0)
			if end <= 0 {
				continue
			}
			// nameZ keeps the NUL terminator that openat requires
			nameZ = nameZ[:end+1]
			name := nameZ[:end]

			switch entryType {
			case unix.DT_DIR:
				if !isDotOrDotDot(name) {
					w.visitDir(dirfd, depth, nameZ, name)
				}
			case unix.DT_REG:
				w.visitFile(dirfd, nameZ, name)
			}
		}
	}
}

func (w *systemControlsWalker) visitDir(dirfd int, depth int, nameZ, name []byte) {
	fd, err := openat(dirfd, nameZ, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC)
	if err != nil {
		return
	}
	defer unix.Close(fd)

	parentLen := w.pushName(name)
	_ = w.walkDir(fd, depth+1) // the entries walked before an error were reported
	w.name = w.name[:parentLen]
}

func (w *systemControlsWalker) visitFile(dirfd int, nameZ, name []byte) {
	fd, err := openat(dirfd, nameZ, unix.O_RDONLY|unix.O_CLOEXEC)
	if err != nil {
		return
	}
	defer unix.Close(fd)

	if err := unix.Fstat(fd, &w.stat); err != nil || w.stat.Mode&0444 == 0 {
		return
	}

	value := redactedValue
	if !w.isIgnored(name) {
		var ok bool
		if value, ok = w.read(fd); !ok {
			return
		}
	}

	parentLen := w.pushName(name)
	w.fn(w.name, bytes.TrimSpace(value))
	w.name = w.name[:parentLen]
}

// read reads the file until its end, in the value buffer, which grows as
// needed and is kept for the next files
func (w *systemControlsWalker) read(fd int) ([]byte, bool) {
	n := 0
	for {
		if n == len(w.value) {
			grown := make([]byte, 2*len(w.value))
			copy(grown, w.value)
			w.value = grown
		}
		r, err := unix.Read(fd, w.value[n:])
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return nil, false
		}
		if r == 0 {
			return w.value[:n], true
		}
		n += r
	}
}

func (w *systemControlsWalker) pushName(name []byte) int {
	parentLen := len(w.name)
	if parentLen > 0 {
		w.name = append(w.name, '/')
	}
	w.name = append(w.name, name...)
	return parentLen
}

func (w *systemControlsWalker) isIgnored(name []byte) bool {
	for _, ignored := range w.ignoredBaseNames {
		if string(name) == ignored {
			return true
		}
	}
	return false
}

func isDotOrDotDot(name []byte) bool {
	return (len(name) == 1 && name[0] == '.') || (len(name) == 2 && name[0] == '.' && name[1] == '.')
}

// openat opens nameZ, a NUL-terminated name relative to dirfd, without the
// allocation that converting a Go string to a C string requires
func openat(dirfd int, nameZ []byte, flags int) (int, error) {
	for {
		fd, _, errno := unix.Syscall6(unix.SYS_OPENAT, uintptr(dirfd), uintptr(unsafe.Pointer(&nameZ[0])), uintptr(flags), 0, 0, 0)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return -1, errno
		}
		return int(fd), nil
	}
}

const stringArenaChunkSize = 32 * 1024

// StringArena copies the names and values reported by WalkProcSys into shared
// chunks, so that keeping them doesn't take two allocations per parameter. The
// copies are immutable and stay valid as long as they are referenced, e.g. by a
// variable that a `set` action assigned from a field of a replayed event.
type StringArena struct {
	chunk []byte
}

// Copy returns copies of name and value
func (a *StringArena) Copy(name, value []byte) (string, string) {
	size := len(name) + len(value)
	if size == 0 {
		return "", ""
	}
	if size > cap(a.chunk)-len(a.chunk) {
		a.chunk = make([]byte, 0, max(stringArenaChunkSize, size))
	}

	start := len(a.chunk)
	a.chunk = append(a.chunk, name...)
	a.chunk = append(a.chunk, value...)
	// the bytes after start are never written again: the chunk is only appended
	// to, within its capacity, or replaced
	copied := a.chunk[start:]
	s := unsafe.String(unsafe.SliceData(copied), len(copied))
	return s[:len(name)], s[len(name):]
}
