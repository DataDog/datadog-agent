// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// recordingPool is a ScanPool that records the submitted jobs. When reject is set, it behaves like
// a full queue: it releases the hash, calls Done and returns false.
type recordingPool struct {
	deduper Deduper
	reject  bool

	mu    sync.Mutex
	jobs  []ScanJob
	sums  [][32]byte
	datas [][]byte
}

func (p *recordingPool) Submit(job ScanJob) bool {
	p.mu.Lock()
	p.jobs = append(p.jobs, job)
	p.sums = append(p.sums, job.Sum)
	p.datas = append(p.datas, bytes.Clone(job.Data))
	p.mu.Unlock()

	if p.reject {
		p.deduper.ReleaseHash(job.Sum)
	}
	if job.Done != nil {
		job.Done()
	}
	return !p.reject
}

func (p *recordingPool) QueueDepth() int {
	return 0
}

func (p *recordingPool) submitted() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.jobs)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type readerFixture struct {
	deduper *LRUDeduper
	pool    *recordingPool
	stats   *Stats
	clock   *fakeClock
	reader  *FileReader
}

func newReaderFixture(t *testing.T, opts FileReaderOpts) *readerFixture {
	t.Helper()
	deduper, err := NewLRUDeduper(100, 100, time.Hour)
	require.NoError(t, err)

	fx := &readerFixture{
		deduper: deduper,
		pool:    &recordingPool{deduper: deduper},
		stats:   &Stats{},
		clock:   &fakeClock{now: time.Unix(1000, 0)},
	}
	opts.Now = fx.clock.Now
	fx.reader = NewFileReader(deduper, fx.pool, fx.stats, opts)
	return fx
}

func (fx *readerFixture) readErrors() map[string]int64 {
	res := make(map[string]int64)
	for i := range fx.stats.ReadErrors {
		if v := fx.stats.ReadErrors[i].Load(); v != 0 {
			res[ReadErrorReason(i).String()] = v
		}
	}
	return res
}

func writeTempFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, content, 0o755))
	return path
}

// scriptExec returns an ExecFile opened through /proc/<test pid>/root/<path>: IsScript skips
// /proc/<pid>/exe, which would point at the test binary. The container ID prevents the pid 1
// fallback of host processes. It carries the real inode and ctime of path, like an exec event
// would, so that it passes the identity check of the path fallback. A missing file gets a zero
// inode and ctime.
func scriptExec(t *testing.T, path string) ExecFile {
	t.Helper()
	f := ExecFile{
		PID:         uint32(os.Getpid()),
		ContainerID: "test-container",
		Path:        path,
		MountID:     1,
		IsScript:    true,
	}
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err == nil {
		f.Inode = st.Ino
		f.CTime = uint64(syscall.TimespecToNsec(st.Ctim))
	}
	return f
}

func fileCTime(t *testing.T, path string) int64 {
	t.Helper()
	var st syscall.Stat_t
	require.NoError(t, syscall.Stat(path, &st))
	return syscall.TimespecToNsec(st.Ctim)
}

// waitCTimeChange chmods path until its ctime differs from before. The kernel updates the ctime
// with a coarse clock, so two changes in the same tick may leave it unchanged.
func waitCTimeChange(t *testing.T, path string, before int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for fileCTime(t, path) == before {
		require.True(t, time.Now().Before(deadline), "ctime of %s didn't change", path)
		time.Sleep(time.Millisecond)
		require.NoError(t, os.Chmod(path, 0o755))
	}
}

// changeCTime changes the ctime of path, like a chmod, without changing its content
func changeCTime(t *testing.T, path string) {
	t.Helper()
	before := fileCTime(t, path)
	require.NoError(t, os.Chmod(path, 0o755))
	waitCTimeChange(t, path, before)
}

// rewriteFile rewrites path in place (same inode), and ensures its ctime changed
func rewriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	before := fileCTime(t, path)
	require.NoError(t, os.WriteFile(path, content, 0o755))
	waitCTimeChange(t, path, before)
}

func TestFileReaderDoneWhen(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	dir := t.TempDir()
	content := []byte("#!/bin/sh\necho v1\n")
	path := writeTempFile(t, dir, "bin", content)
	f := scriptExec(t, path)

	// the same binary executed 10,000 times is read once
	for i := 0; i < 10000; i++ {
		fx.reader.Process(f)
	}
	assert.EqualValues(t, 1, fx.stats.Reads.Load())
	assert.EqualValues(t, 9999, fx.stats.IdentityHits.Load())
	require.Equal(t, 1, fx.pool.submitted())
	assert.Equal(t, sha256.Sum256(content), fx.pool.sums[0])
	assert.Equal(t, content, fx.pool.datas[0])
	assert.Equal(t, f, fx.pool.jobs[0].File)

	// rewriting the binary changes its ctime: exactly one more read and one more scan
	rewritten := []byte("#!/bin/sh\necho v2\n")
	rewriteFile(t, path, rewritten)
	f = scriptExec(t, path)
	for i := 0; i < 10000; i++ {
		fx.reader.Process(f)
	}
	assert.EqualValues(t, 2, fx.stats.Reads.Load())
	require.Equal(t, 2, fx.pool.submitted())
	assert.Equal(t, sha256.Sum256(rewritten), fx.pool.sums[1])
	assert.Equal(t, rewritten, fx.pool.datas[1])

	// the same content at another path and identity is read, but not scanned again
	copyPath := writeTempFile(t, dir, "copy", rewritten)
	fx.reader.Process(scriptExec(t, copyPath))
	assert.EqualValues(t, 3, fx.stats.Reads.Load())
	assert.EqualValues(t, 1, fx.stats.ShaHits.Load())
	assert.Equal(t, 2, fx.pool.submitted())

	identities, hashes := fx.deduper.Sizes()
	assert.Equal(t, 3, identities)
	assert.Equal(t, 2, hashes)
	assert.Empty(t, fx.readErrors())
}

func TestFileReaderCTimeChangeSameContent(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
	f := scriptExec(t, path)

	fx.reader.Process(f)
	changeCTime(t, path) // e.g. chmod: new identity, same content
	fx.reader.Process(scriptExec(t, path))

	assert.EqualValues(t, 2, fx.stats.Reads.Load())
	assert.EqualValues(t, 1, fx.stats.ShaHits.Load())
	assert.Equal(t, 1, fx.pool.submitted())
}

func TestFileReaderTTLExpiry(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
	f := scriptExec(t, path)

	fx.reader.Process(f)
	fx.clock.Advance(59 * time.Minute)
	fx.reader.Process(f)
	assert.EqualValues(t, 1, fx.stats.Reads.Load())
	assert.EqualValues(t, 1, fx.stats.IdentityHits.Load())

	// past the TTL the file is read again; the content is unchanged so it isn't rescanned
	fx.clock.Advance(time.Minute)
	fx.reader.Process(f)
	assert.EqualValues(t, 2, fx.stats.Reads.Load())
	assert.EqualValues(t, 1, fx.stats.ShaHits.Load())
	assert.Equal(t, 1, fx.pool.submitted())

	// the identity was re-marked
	fx.reader.Process(f)
	assert.EqualValues(t, 2, fx.stats.Reads.Load())
	assert.EqualValues(t, 2, fx.stats.IdentityHits.Load())
}

func TestFileReaderIdentityBypass(t *testing.T) {
	for _, fsType := range []string{"fuse", "fuseblk", "fuse.sshfs", "nfs", "nfs4", "cifs", "smb3", "9p"} {
		t.Run(fsType, func(t *testing.T) {
			fx := newReaderFixture(t, FileReaderOpts{})
			path := writeTempFile(t, t.TempDir(), "bin", []byte("v1"))
			f := scriptExec(t, path)
			f.Filesystem = fsType

			fx.reader.Process(f)
			fx.reader.Process(f)

			// a remote write that the local ctime doesn't reflect is still caught: f keeps the old
			// ctime, which the identity check ignores on these filesystems
			require.NoError(t, os.WriteFile(path, []byte("v2"), 0o755))
			fx.reader.Process(f)

			assert.EqualValues(t, 3, fx.stats.Reads.Load())
			assert.EqualValues(t, 0, fx.stats.IdentityHits.Load())
			assert.EqualValues(t, 1, fx.stats.ShaHits.Load())
			assert.Equal(t, 2, fx.pool.submitted())

			identities, _ := fx.deduper.Sizes()
			assert.Equal(t, 0, identities, "bypassed identities are not cached")
		})
	}
}

func TestBypassIdentityCache(t *testing.T) {
	for _, fsType := range []string{"", "ext4", "xfs", "overlay", "tmpfs", "btrfs"} {
		assert.False(t, bypassIdentityCache(fsType), fsType)
	}
}

func TestFileReaderPoolDropReleasesHash(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
	f := scriptExec(t, path)

	fx.pool.reject = true
	fx.reader.Process(f)
	require.Equal(t, 1, fx.pool.submitted())
	_, hashes := fx.deduper.Sizes()
	assert.Equal(t, 0, hashes, "a dropped scan releases its hash")

	// the next exec of the same content at another identity is scanned
	fx.pool.reject = false
	changeCTime(t, path)
	fx.reader.Process(scriptExec(t, path))
	assert.Equal(t, 2, fx.pool.submitted())
	assert.EqualValues(t, 0, fx.stats.ShaHits.Load())
	_, hashes = fx.deduper.Sizes()
	assert.Equal(t, 1, hashes)
}

func TestFileReaderTooBig(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{MaxFileSize: 10})
	dir := t.TempDir()

	small := scriptExec(t, writeTempFile(t, dir, "small", []byte("0123456789")))
	fx.reader.Process(small)
	assert.EqualValues(t, 1, fx.stats.Reads.Load())

	big := scriptExec(t, writeTempFile(t, dir, "big", []byte("0123456789a")))
	fx.reader.Process(big)
	fx.reader.Process(big)
	assert.EqualValues(t, 1, fx.stats.TooBig.Load(), "too big is permanent for an identity")
	assert.EqualValues(t, 1, fx.stats.IdentityHits.Load())
	assert.EqualValues(t, 1, fx.stats.Reads.Load())
	assert.Equal(t, 1, fx.pool.submitted())
}

func TestFileReaderNotRegular(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	dir := t.TempDir()

	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))

	done := make(chan struct{})
	go func() {
		defer close(done)
		fx.reader.Process(scriptExec(t, fifo))
		fx.reader.Process(scriptExec(t, dir))
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("opening a FIFO blocked")
	}

	assert.EqualValues(t, 2, fx.stats.NotRegular.Load())
	assert.EqualValues(t, 0, fx.stats.Reads.Load())
	assert.Equal(t, 0, fx.pool.submitted())
}

func TestFileReaderNotFoundIsRetried(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	path := filepath.Join(t.TempDir(), "missing")

	fx.reader.Process(scriptExec(t, path))
	assert.Equal(t, map[string]int64{"not_found": 1}, fx.readErrors())
	identities, _ := fx.deduper.Sizes()
	assert.Equal(t, 0, identities)

	// open errors don't mark the identity, so the next exec retries
	require.NoError(t, os.WriteFile(path, []byte("content"), 0o755))
	fx.reader.Process(scriptExec(t, path))
	assert.EqualValues(t, 1, fx.stats.Reads.Load())
	assert.Equal(t, 1, fx.pool.submitted())
}

func TestFileReaderPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	fx := newReaderFixture(t, FileReaderOpts{})
	path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
	require.NoError(t, os.Chmod(path, 0o100))

	fx.reader.Process(scriptExec(t, path))
	assert.Equal(t, map[string]int64{"permission": 1}, fx.readErrors())
}

func TestFileReaderContainerPIDsFallback(t *testing.T) {
	var lookups []containerutils.ContainerID
	fx := newReaderFixture(t, FileReaderOpts{
		ContainerPIDs: func(id containerutils.ContainerID) []uint32 {
			lookups = append(lookups, id)
			return []uint32{uint32(os.Getpid())}
		},
	})
	path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
	f := scriptExec(t, path)
	// a pid that doesn't exist: the exec'ing process is gone
	f.PID = 1 << 30

	fx.reader.Process(f)
	assert.Equal(t, []containerutils.ContainerID{"test-container"}, lookups)
	assert.EqualValues(t, 1, fx.stats.Reads.Load())
	assert.Equal(t, 1, fx.pool.submitted())
	assert.Empty(t, fx.readErrors())
}

func TestFileReaderProcExe(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	exe, err := os.Executable()
	require.NoError(t, err)
	content, err := os.ReadFile(exe)
	require.NoError(t, err)

	// the path doesn't exist in the mount namespace: /proc/<pid>/exe is used. Its identity is not
	// checked (Inode is wrong here): it is the executed inode itself.
	fx.reader.Process(ExecFile{
		PID:         uint32(os.Getpid()),
		ContainerID: "test-container",
		Path:        "/does/not/exist",
		Inode:       1,
	})
	require.Equal(t, 1, fx.pool.submitted())
	assert.Equal(t, sha256.Sum256(content), fx.pool.sums[0])
}

func TestCandidatePaths(t *testing.T) {
	r := NewFileReader(NewStandInDeduper(), &recordingPool{}, &Stats{}, FileReaderOpts{
		ContainerPIDs: func(containerutils.ContainerID) []uint32 { return []uint32{10, 11, 12} },
	})

	exe := func(pid uint32) candidatePath { return candidatePath{path: utils.ProcExePath(pid)} }
	root := func(pid uint32) candidatePath {
		return candidatePath{path: utils.ProcRootFilePath(pid, "/usr/bin/true"), verify: true}
	}

	f := ExecFile{PID: 11, ContainerID: "c", Path: "/usr/bin/true"}
	assert.Equal(t, []candidatePath{exe(11), root(11), root(10), root(12)}, r.candidatePaths(&f))

	f.IsScript = true
	assert.Equal(t, []candidatePath{root(11), root(10), root(12)}, r.candidatePaths(&f))

	host := ExecFile{PID: 11, Path: "/usr/bin/true"}
	assert.Equal(t, []candidatePath{exe(11), root(11), root(1)}, r.candidatePaths(&host))

	noLookup := NewFileReader(NewStandInDeduper(), &recordingPool{}, &Stats{}, FileReaderOpts{})
	f.IsScript = false
	assert.Equal(t, []candidatePath{exe(11), root(11)}, noLookup.candidatePaths(&f))
}

func TestFileReaderIdentityMismatch(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate     func(f *ExecFile)
		filesystem string
		mismatch   bool
	}{
		"match":                      {mutate: func(*ExecFile) {}},
		"inode differs":              {mutate: func(f *ExecFile) { f.Inode++ }, mismatch: true},
		"ctime differs":              {mutate: func(f *ExecFile) { f.CTime++ }, mismatch: true},
		"no ctime in the event":      {mutate: func(f *ExecFile) { f.CTime = 0 }},
		"inode differs on nfs":       {mutate: func(f *ExecFile) { f.Inode++ }, filesystem: "nfs", mismatch: true},
		"ctime ignored on nfs":       {mutate: func(f *ExecFile) { f.CTime++ }, filesystem: "nfs"},
		"mount id can't be compared": {mutate: func(f *ExecFile) { f.MountID += 10 }},
	} {
		t.Run(name, func(t *testing.T) {
			fx := newReaderFixture(t, FileReaderOpts{})
			path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
			f := scriptExec(t, path)
			f.Filesystem = tc.filesystem
			tc.mutate(&f)

			fx.reader.Process(f)
			if !tc.mismatch {
				assert.EqualValues(t, 1, fx.stats.Reads.Load())
				assert.Equal(t, 1, fx.pool.submitted())
				assert.Empty(t, fx.readErrors())
				return
			}
			identities, _ := fx.deduper.Sizes()
			assert.EqualValues(t, 0, fx.stats.Reads.Load())
			assert.Equal(t, 0, fx.pool.submitted(), "not scanned under the event's identity")
			assert.Equal(t, map[string]int64{"other": 1}, fx.readErrors())
			assert.Equal(t, 0, identities, "the identity is not marked")
		})
	}
}

func TestFileReaderPathReplaced(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	dir := t.TempDir()
	path := writeTempFile(t, dir, "bin", []byte("executed"))
	f := scriptExec(t, path)

	// the path is replaced between the exec and the open
	replacement := writeTempFile(t, dir, "replacement", []byte("benign"))
	require.NoError(t, os.Rename(replacement, path))

	fx.reader.Process(f)
	assert.Equal(t, 0, fx.pool.submitted())
	assert.Equal(t, map[string]int64{"other": 1}, fx.readErrors())

	// the next exec, of the new file, is read and scanned
	fx.reader.Process(scriptExec(t, path))
	require.Equal(t, 1, fx.pool.submitted())
	assert.Equal(t, []byte("benign"), fx.pool.datas[0])
}

func TestFileReaderContainerPIDsMismatch(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{
		ContainerPIDs: func(containerutils.ContainerID) []uint32 { return []uint32{uint32(os.Getpid())} },
	})
	path := writeTempFile(t, t.TempDir(), "bin", []byte("content"))
	f := scriptExec(t, path)
	f.PID = 1 << 30 // gone: only the other container pid is tried
	f.Inode++

	fx.reader.Process(f)
	assert.Equal(t, 0, fx.pool.submitted())
	assert.Equal(t, map[string]int64{"other": 1}, fx.readErrors(), "a mismatch wins over not found")
}

func TestReadAll(t *testing.T) {
	content := []byte(strings.Repeat("0123456789", 100))

	// growing from a small buffer, one byte per read
	data, err := readAll(iotest.OneByteReader(bytes.NewReader(content)), make([]byte, 0, 3), 0)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	data, err = readAll(bytes.NewReader(content), nil, int64(len(content)))
	require.NoError(t, err)
	assert.Equal(t, content, data)

	_, err = readAll(bytes.NewReader(content), nil, int64(len(content)-1))
	assert.ErrorIs(t, err, errTooBig)

	_, err = readAll(iotest.ErrReader(syscall.EIO), nil, 0)
	assert.ErrorIs(t, err, syscall.EIO)
}

func TestFileReaderBuffersReused(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	var done int
	pool := &countingPool{onSubmit: func(job ScanJob) {
		job.Done()
		done++
	}}
	fx.reader.pool = pool

	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		path := writeTempFile(t, dir, "bin"+string(rune('0'+i)), []byte(strings.Repeat("x", i+1)))
		fx.reader.Process(scriptExec(t, path))
	}
	assert.Equal(t, 5, done)
	assert.EqualValues(t, 5, fx.stats.Reads.Load())
}

type countingPool struct {
	onSubmit func(job ScanJob)
}

func (p *countingPool) Submit(job ScanJob) bool {
	p.onSubmit(job)
	return true
}

func (p *countingPool) QueueDepth() int {
	return 0
}

func TestFileReaderConcurrent(t *testing.T) {
	fx := newReaderFixture(t, FileReaderOpts{})
	dir := t.TempDir()

	// 4 identities, 2 distinct contents
	var files []ExecFile
	for i := 0; i < 4; i++ {
		content := []byte{byte('a' + i%2)}
		path := writeTempFile(t, dir, "bin"+string(rune('0'+i)), content)
		files = append(files, scriptExec(t, path))
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i := 0; i < 200; i++ {
				fx.reader.Process(files[(g+i)%len(files)])
			}
		}(g)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 2, fx.pool.submitted(), "one scan per distinct content")
	reads := fx.stats.Reads.Load()
	assert.GreaterOrEqual(t, reads, int64(4))
	assert.EqualValues(t, 16*200, reads+fx.stats.IdentityHits.Load())
	assert.EqualValues(t, reads-2, fx.stats.ShaHits.Load())
}
