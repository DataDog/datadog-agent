// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// maxPooledBufferSize is the largest buffer returned to the pool. Larger buffers are left to
// the GC, so that a few huge binaries don't pin that much memory between scans.
const maxPooledBufferSize = 16 * 1024 * 1024

// ContainerPIDsFunc returns the PIDs of the processes running in a container. It is used to find
// a mount namespace where the file is still reachable after the exec'ing process exited.
type ContainerPIDsFunc func(containerID containerutils.ContainerID) []uint32

// FileReaderOpts are the options of a FileReader
type FileReaderOpts struct {
	// MaxFileSize is the size above which files are skipped. <= 0 means no limit.
	MaxFileSize int64
	// ContainerPIDs is optional. When nil, only the exec'ing process (and pid 1 for host
	// processes) is used to open the file.
	ContainerPIDs ContainerPIDsFunc
	// Now is optional, and defaults to time.Now. It is used for the identity TTL.
	Now func() time.Time
}

// FileReader is the pipeline stage between the exec consumer and the scan pool: it filters out
// fresh identities, reads the file once, hashes it, and submits unseen contents to the pool.
// It is safe for concurrent use.
type FileReader struct {
	deduper       Deduper
	pool          ScanPool
	stats         *Stats
	maxFileSize   int64
	containerPIDs ContainerPIDsFunc
	now           func() time.Time

	buffers sync.Pool
}

// NewFileReader returns a new FileReader. deduper, pool and stats must not be nil.
func NewFileReader(deduper Deduper, pool ScanPool, stats *Stats, opts FileReaderOpts) *FileReader {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &FileReader{
		deduper:       deduper,
		pool:          pool,
		stats:         stats,
		maxFileSize:   opts.MaxFileSize,
		containerPIDs: opts.ContainerPIDs,
		now:           now,
	}
}

// readOutcome is the result of reading an exec'd file
type readOutcome int

const (
	// readOK means the whole content was read and hashed
	readOK readOutcome = iota
	// readSkipped means the file will never be read at this identity (not regular, too big)
	readSkipped
	// readFailed means an open, stat or read error that may be transient
	readFailed
)

// Process handles an exec'd file: identity dedupe, open, read, hash, content dedupe, submit.
//
// The identity is marked once the content was read, and when the file is permanently skipped
// (not regular, too big): those outcomes can only change with a new ctime. It is not marked on
// open or read errors, which may be transient, so that a later exec retries. Two concurrent
// execs of a new identity may both read the file; the content level still ensures one scan.
//
// On FUSE and network filesystems the identity is neither checked nor marked: the ctime seen
// by the kernel may not reflect remote writes, so every exec is read and hashed.
func (r *FileReader) Process(f ExecFile) {
	id := f.Identity()
	useIdentity := !bypassIdentityCache(f.Filesystem)

	if useIdentity && r.deduper.IdentityFresh(id, r.now()) {
		r.stats.IdentityHits.Add(1)
		return
	}

	buf, sum, outcome := r.read(&f)
	if useIdentity && outcome != readFailed {
		r.deduper.MarkIdentity(id, r.now())
	}
	if outcome != readOK {
		return
	}

	if !r.deduper.ClaimHash(sum) {
		r.stats.ShaHits.Add(1)
		r.putBuffer(buf)
		return
	}

	var once sync.Once
	job := ScanJob{
		File: f,
		Sum:  sum,
		Data: *buf,
		Done: func() {
			once.Do(func() { r.putBuffer(buf) })
		},
	}
	// on false, the pool has already released the hash and called Done
	_ = r.pool.Submit(job)
}

// read opens the file, reads it whole into a pooled buffer and hashes it. The buffer is only
// returned with readOK, and the caller then owns it.
func (r *FileReader) read(f *ExecFile) (*[]byte, [32]byte, readOutcome) {
	var sum [32]byte

	file, err := r.open(f)
	if err != nil {
		r.stats.IncReadError(openErrorReason(err))
		return nil, sum, readFailed
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		r.stats.IncReadError(ReadErrorOther)
		return nil, sum, readFailed
	}
	if !info.Mode().IsRegular() {
		r.stats.NotRegular.Add(1)
		return nil, sum, readSkipped
	}
	if r.maxFileSize > 0 && info.Size() > r.maxFileSize {
		r.stats.TooBig.Add(1)
		return nil, sum, readSkipped
	}

	buf := r.getBuffer(info.Size())
	data, err := readAll(file, *buf, r.maxFileSize)
	*buf = data
	if err != nil {
		r.putBuffer(buf)
		if errors.Is(err, errTooBig) {
			// the file grew past the limit after fstat
			r.stats.TooBig.Add(1)
			return nil, sum, readSkipped
		}
		r.stats.IncReadError(ReadErrorOther)
		return nil, sum, readFailed
	}

	r.stats.Reads.Add(1)
	sum = sha256.Sum256(data)
	return buf, sum, readOK
}

// open opens the first candidate path that works. See candidatePaths for the order.
func (r *FileReader) open(f *ExecFile) (*os.File, error) {
	var lastErr error
	for _, path := range r.candidatePaths(f) {
		// O_NONBLOCK so that opening a FIFO found at the path doesn't block; it has no effect on
		// regular files. Non-regular files are then rejected after fstat.
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err == nil {
			return file, nil
		}
		// keep the most informative error: anything but "not found" wins
		if !isNotFound(err) || lastErr == nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		return nil, os.ErrNotExist
	}
	return nil, lastErr
}

// candidatePaths returns the paths to try, in order:
//  1. /proc/<pid>/exe, the exact executed inode, even if it was unlinked or replaced. Skipped
//     for scripts, since it points at the interpreter.
//  2. /proc/<pid>/root/<path>, the path in the process' mount namespace.
//  3. /proc/<other pid>/root/<path>, for the other processes of the same container, in case the
//     exec'ing process is gone. For host processes, pid 1 is used.
func (r *FileReader) candidatePaths(f *ExecFile) []string {
	paths := make([]string, 0, 4)
	if !f.IsScript && f.PID != 0 {
		paths = append(paths, utils.ProcExePath(f.PID))
	}
	if f.Path == "" {
		return paths
	}
	if f.PID != 0 {
		paths = append(paths, utils.ProcRootFilePath(f.PID, f.Path))
	}

	var others []uint32
	if f.ContainerID == "" {
		others = []uint32{1}
	} else if r.containerPIDs != nil {
		others = r.containerPIDs(f.ContainerID)
	}
	for _, pid := range others {
		if pid == f.PID || pid == 0 {
			continue
		}
		paths = append(paths, utils.ProcRootFilePath(pid, f.Path))
	}
	return paths
}

// getBuffer returns an empty buffer from the pool, able to hold size bytes plus one, so that
// readAll can detect EOF without growing it
func (r *FileReader) getBuffer(size int64) *[]byte {
	want := int(size) + 1
	if v := r.buffers.Get(); v != nil {
		buf := v.(*[]byte)
		if cap(*buf) >= want {
			*buf = (*buf)[:0]
			return buf
		}
		// too small for this file: put it back for a smaller one
		r.buffers.Put(buf)
	}
	buf := make([]byte, 0, want)
	return &buf
}

// putBuffer returns a buffer to the pool
func (r *FileReader) putBuffer(buf *[]byte) {
	if buf == nil || cap(*buf) > maxPooledBufferSize {
		return
	}
	*buf = (*buf)[:0]
	r.buffers.Put(buf)
}

var errTooBig = errors.New("file is larger than the maximum file size")

// readAll reads r until EOF, appending to buf. It returns errTooBig once more than limit bytes
// were read, when limit > 0. Reading until EOF, rather than fstat's size, ensures the hashed
// bytes are the whole file even if it was being written to.
func readAll(r io.Reader, buf []byte, limit int64) ([]byte, error) {
	for {
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if limit > 0 && int64(len(buf)) > limit {
			return buf, errTooBig
		}
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return buf, err
		}
	}
}

func isNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

// openErrorReason classifies an open error
func openErrorReason(err error) ReadErrorReason {
	switch {
	case isNotFound(err):
		return ReadErrorNotFound
	case errors.Is(err, os.ErrPermission):
		return ReadErrorPermission
	default:
		return ReadErrorOther
	}
}

// bypassIdentityCache returns true for the filesystems where the (mount, inode, ctime) identity
// can't be trusted to change when the content does: FUSE (the daemon controls the attributes)
// and network filesystems (writes from other clients).
func bypassIdentityCache(fsType string) bool {
	if fsType == "" {
		return false
	}
	if strings.HasPrefix(fsType, "fuse") {
		// fuse, fuseblk, fuse.<subtype>
		return true
	}
	switch fsType {
	case "nfs", "nfs4", "cifs", "smb", "smb3", "9p", "ceph", "afs", "glusterfs":
		return true
	}
	return false
}
