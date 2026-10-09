// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package fake provides an in-memory SMB share for tests of the SMB launcher
// and tailers. A Share holds the files and lets the test act as the log
// writer (append, rotate, truncate, delete); Sessions are the client.Client
// connections the code under test reads through.
//
// The fake follows the server behavior the source depends on: a FileID and a
// creation time survive renames and truncation, a delete and recreate gives
// the file a new creation time and a new FileID (unless the test reuses the
// FileID on purpose, as Samba reuses inode numbers), directory listings can
// report stale sizes, and errors classify with client.Classify like the real
// ones.
package fake

import (
	"context"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
)

// Errors with the server status codes the real client sees. Inject them with
// FailNext or FailNextPath; client.Classify maps each to the kind in its name.
var (
	ErrTransient    error = &protocol.ResponseError{Code: 0xC000035C} // STATUS_NETWORK_SESSION_EXPIRED
	ErrOverloaded   error = &protocol.ResponseError{Code: 0xC0000205} // STATUS_INSUFF_SERVER_RESOURCES (ErrTransient kind)
	ErrNotFound     error = &protocol.ResponseError{Code: 0xC0000034} // STATUS_OBJECT_NAME_NOT_FOUND
	ErrAuth         error = &protocol.ResponseError{Code: 0xC000006D} // STATUS_LOGON_FAILURE
	ErrAccessDenied error = &protocol.ResponseError{Code: 0xC0000022} // STATUS_ACCESS_DENIED (ErrAuth kind)
	ErrLockedOut    error = &protocol.ResponseError{Code: 0xC0000234} // STATUS_ACCOUNT_LOCKED_OUT
	ErrDisabled     error = &protocol.ResponseError{Code: 0xC0000072} // STATUS_ACCOUNT_DISABLED
	ErrSharing      error = &protocol.ResponseError{Code: 0xC0000043} // STATUS_SHARING_VIOLATION
	errNotADir      error = &protocol.ResponseError{Code: 0xC0000103} // STATUS_NOT_A_DIRECTORY
	errIsADir       error = &protocol.ResponseError{Code: 0xC00000BA} // STATUS_FILE_IS_A_DIRECTORY
)

// Op names a client call, for error injection, hooks and call counts.
type Op int

const (
	OpDial Op = iota
	OpListDir
	OpReadAt
	// OpLogoff is a Session.Close, which logs the session off. Only Calls
	// counts it: it takes no hook and no injected error.
	OpLogoff
)

func (op Op) String() string {
	switch op {
	case OpDial:
		return "dial"
	case OpListDir:
		return "readdir"
	case OpReadAt:
		return "read"
	case OpLogoff:
		return "logoff"
	default:
		return fmt.Sprintf("op(%d)", int(op))
	}
}

type file struct {
	id         uint64
	data       []byte
	listedSize int64 // size reported by ListDir while stale listing is on
	ctime      time.Time
	mtime      time.Time
}

type errKey struct {
	op   Op
	path string
	any  bool
}

// Share is an in-memory share. All methods are safe for concurrent use.
// Paths use the client's form (see client.CleanPath); the methods that act as
// the writer panic on an invalid path, since that is a bug in the test.
type Share struct {
	mu       sync.Mutex
	files    map[string]*file
	dirs     map[string]bool
	nextID   uint64
	created  time.Time // creation time of the last file created
	stale    bool
	listIDs  bool
	readIDs  bool
	ctimes   bool
	latency  time.Duration
	logoff   time.Duration
	hook     func(op Op, path string)
	errs     map[errKey][]error
	calls    map[Op]int
	open     int
	sessions map[*Session]bool // live (not closed) sessions -> dropped
}

// New returns an empty share. FileIDs start at 100, so a FileID in a test
// failure is easy to tell apart from an offset or a size.
func New() *Share {
	return &Share{
		files:    make(map[string]*file),
		dirs:     make(map[string]bool),
		nextID:   100,
		listIDs:  true,
		readIDs:  true,
		ctimes:   true,
		errs:     make(map[errKey][]error),
		calls:    make(map[Op]int),
		sessions: make(map[*Session]bool),
	}
}

// --- writer side ----------------------------------------------------------

// Mkdir creates dir and its parents.
func (s *Share) Mkdir(dir string) {
	dir = mustClean(dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addDirs(dir)
}

// Rmdir removes dir, and every directory and file under it.
func (s *Share) Rmdir(dir string) {
	dir = mustClean(dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := dir + "/"
	for p := range s.files {
		if p == dir || strings.HasPrefix(p, prefix) {
			delete(s.files, p)
		}
	}
	for d := range s.dirs {
		if d == dir || strings.HasPrefix(d, prefix) {
			delete(s.dirs, d)
		}
	}
}

// Write replaces the content of p, creating it (and its parent directories)
// with a new FileID if needed, and returns the FileID.
func (s *Share) Write(p string, data []byte) uint64 {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(p)
	f.data = slices.Clone(data)
	f.mtime = time.Now()
	return f.id
}

// Append appends data to p, creating it if needed, and returns its FileID.
func (s *Share) Append(p string, data []byte) uint64 {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(p)
	f.data = append(f.data, data...)
	f.mtime = time.Now()
	return f.id
}

// Rename moves oldPath to newPath, keeping its FileID and replacing any file
// already at newPath, like a rotation by rename.
func (s *Share) Rename(oldPath, newPath string) error {
	oldPath, newPath = mustClean(oldPath), mustClean(newPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[oldPath]
	if !ok {
		return &os.PathError{Op: "rename", Path: oldPath, Err: ErrNotFound}
	}
	if s.isDir(newPath) {
		return &os.PathError{Op: "rename", Path: newPath, Err: errIsADir}
	}
	delete(s.files, oldPath)
	s.files[newPath] = f
	s.addDirs(path.Dir(newPath))
	return nil
}

// Truncate sets the size of p, keeping its FileID (copytruncate). Growing
// pads with zero bytes.
func (s *Share) Truncate(p string, size int64) error {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[p]
	if !ok {
		return &os.PathError{Op: "truncate", Path: p, Err: ErrNotFound}
	}
	if size < int64(len(f.data)) {
		f.data = f.data[:size]
	} else {
		f.data = append(f.data, make([]byte, size-int64(len(f.data)))...)
	}
	f.mtime = time.Now()
	return nil
}

// Delete removes p. Its FileID is not reused unless a test passes it to
// Recreate.
func (s *Share) Delete(p string) error {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.files[p]; !ok {
		return &os.PathError{Op: "delete", Path: p, Err: ErrNotFound}
	}
	delete(s.files, p)
	return nil
}

// Recreate deletes p if it exists and creates it empty, with a new creation
// time and a new FileID, or with reuseID when it is not 0 (servers such as
// Samba reuse inode numbers). It returns the new FileID.
func (s *Share) Recreate(p string, reuseID uint64) uint64 {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, p)
	f := s.file(p)
	if reuseID != 0 {
		f.id = reuseID
	}
	return f.id
}

// SetModTime sets the last write time p's listing reports, which Write and
// Append otherwise set to the real time. It panics if p does not exist.
func (s *Share) SetModTime(p string, t time.Time) {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[p]
	if !ok {
		panic("fake: SetModTime of missing file " + p)
	}
	f.mtime = t
}

// Stat returns the current, never stale, entry for p.
func (s *Share) Stat(p string) (client.Entry, bool) {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[p]
	if !ok {
		return client.Entry{}, false
	}
	return client.Entry{
		Name:         path.Base(p),
		Size:         int64(len(f.data)),
		FileID:       f.id,
		ModTime:      f.mtime,
		CreationTime: f.ctime,
	}, true
}

// --- server behavior ------------------------------------------------------

// SetStaleListing freezes (true) or unfreezes (false) the sizes reported by
// ListDir. While frozen, ListDir reports each file's size as of the moment
// the listing was frozen, or 0 for files created since, as servers do for
// files another client holds open for writing. ReadAt always reports the
// current size.
func (s *Share) SetStaleListing(stale bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stale && !s.stale {
		for _, f := range s.files {
			f.listedSize = int64(len(f.data))
		}
	}
	s.stale = stale
}

// SetListingFileIDs controls whether ListDir reports FileIDs (default true).
// false mimics a server that returns 0.
func (s *Share) SetListingFileIDs(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listIDs = enabled
}

// SetReadFileIDs controls whether ReadAt reports FileIDs (default true).
// false mimics a server without QFid support.
func (s *Share) SetReadFileIDs(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readIDs = enabled
}

// SetCreationTimes controls whether ListDir and ReadAt report creation times
// (default true). false mimics a server that reports none (a zero FILETIME).
func (s *Share) SetCreationTimes(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctimes = enabled
}

// SetLatency delays every call by d, or until its context ends.
func (s *Share) SetLatency(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latency = d
}

// SetLogoffLatency makes Session.Close wait d before it returns, as a LOGOFF
// to a server that stopped answering does, unless Session.Abort cuts it short.
// The session counts as live while it waits.
func (s *Share) SetLogoffLatency(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logoff = d
}

// SetHook registers fn to run at the start of every call, before any error
// injection, without the share's lock held: fn may change the share, for
// example to rotate a file between a listing and a read.
func (s *Share) SetHook(fn func(op Op, path string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hook = fn
}

// FailNext makes the next len(errs) calls of op fail with errs, in order,
// whatever their path. A nil entry lets that call succeed.
func (s *Share) FailNext(op Op, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := errKey{op: op, any: true}
	s.errs[k] = append(s.errs[k], errs...)
}

// FailNextPath is FailNext for the calls of op on one path (or directory).
// These errors are used before the ones queued by FailNext.
func (s *Share) FailNextPath(op Op, p string, errs ...error) {
	p = mustClean(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	k := errKey{op: op, path: p}
	s.errs[k] = append(s.errs[k], errs...)
}

// DropSessions breaks every open session: their calls fail with ErrTransient
// until they are closed. Sessions dialed afterwards work.
func (s *Share) DropSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sess := range s.sessions {
		s.sessions[sess] = true
	}
}

// Calls returns how many times op was called, including failed calls.
func (s *Share) Calls(op Op) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[op]
}

// OpenHandles returns the number of ReadAt calls in progress, i.e. file
// handles a real server would see open.
func (s *Share) OpenHandles() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

// LiveSessions returns the number of sessions dialed and not closed yet,
// dropped or not.
func (s *Share) LiveSessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// --- client side ----------------------------------------------------------

// Dial is a client.DialFunc: pass it to client.WithDialer. Errors queued for
// OpDial fail the dial. The configuration is ignored.
func (s *Share) Dial(ctx context.Context, _ client.Config) (client.Client, error) {
	if err := s.begin(ctx, OpDial, "", nil); err != nil {
		return nil, err
	}
	return s.NewSession(), nil
}

// NewSession returns a connected session, bypassing dial errors and counts.
func (s *Share) NewSession() *Session {
	sess := &Session{share: s, aborted: make(chan struct{})}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess] = false
	return sess
}

// Session is one connection to a Share. It implements client.Client, and the
// Abort method client.Abort uses.
type Session struct {
	share     *Share
	closed    bool // guarded by share.mu
	aborted   chan struct{}
	abortOnce sync.Once
}

var _ client.Client = (*Session)(nil)

// ListDir implements client.Client.
func (sess *Session) ListDir(ctx context.Context, dir string) ([]client.Entry, error) {
	dir, err := client.CleanPath(dir)
	if err != nil {
		return nil, err
	}
	s := sess.share
	if err := s.begin(ctx, OpListDir, dir, sess); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.files[dir]; ok {
		return nil, &os.PathError{Op: "readdir", Path: dir, Err: errNotADir}
	}
	if !s.isDir(dir) {
		return nil, &os.PathError{Op: "readdir", Path: dir, Err: ErrNotFound}
	}
	var entries []client.Entry
	for p, f := range s.files {
		if path.Dir(p) != dirOf(dir) {
			continue
		}
		e := client.Entry{
			Name:    path.Base(p),
			Size:    int64(len(f.data)),
			ModTime: f.mtime,
		}
		if s.ctimes {
			e.CreationTime = f.ctime
		}
		if s.stale {
			e.Size = f.listedSize
		}
		if s.listIDs {
			e.FileID = f.id
		}
		entries = append(entries, e)
	}
	for d := range s.dirs {
		if d != "" && path.Dir(d) == dirOf(dir) {
			entries = append(entries, client.Entry{Name: path.Base(d), IsDir: true})
		}
	}
	slices.SortFunc(entries, func(a, b client.Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

// ReadAt implements client.Client.
func (sess *Session) ReadAt(ctx context.Context, p string, off int64, maxLen int) (client.ReadResult, error) {
	name, err := client.CleanPath(p)
	if err != nil {
		return client.ReadResult{}, err
	}
	if name == "" || off < 0 || maxLen < 0 {
		return client.ReadResult{}, &os.PathError{Op: "read", Path: p, Err: os.ErrInvalid}
	}
	s := sess.share
	if err := s.begin(ctx, OpReadAt, name, sess); err != nil {
		return client.ReadResult{}, err
	}
	defer func() {
		s.mu.Lock()
		s.open--
		s.mu.Unlock()
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[name]
	if !ok {
		if s.isDir(name) {
			return client.ReadResult{}, &os.PathError{Op: "read", Path: name, Err: errIsADir}
		}
		return client.ReadResult{}, &os.PathError{Op: "read", Path: name, Err: ErrNotFound}
	}
	res := client.ReadResult{Size: int64(len(f.data))}
	if s.readIDs {
		res.FileID = f.id
	}
	if s.ctimes {
		res.CreationTime = f.ctime
	}
	if off < res.Size && maxLen > 0 {
		end := min(res.Size, off+int64(maxLen))
		res.Data = slices.Clone(f.data[off:end])
	}
	return res, nil
}

// Close implements client.Client. It waits for the logoff latency, if any.
func (sess *Session) Close() error {
	s := sess.share
	s.mu.Lock()
	closed := sess.closed
	logoff := s.logoff
	if !closed {
		s.calls[OpLogoff]++
	}
	s.mu.Unlock()
	if !closed && logoff > 0 {
		t := time.NewTimer(logoff)
		defer t.Stop()
		select {
		case <-t.C:
		case <-sess.aborted:
		}
	}
	sess.close()
	return nil
}

// Abort closes the session without logging off: it never waits, and it cuts
// short a Close waiting for its logoff latency.
func (sess *Session) Abort() error {
	sess.abortOnce.Do(func() { close(sess.aborted) })
	sess.close()
	return nil
}

func (sess *Session) close() {
	s := sess.share
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.closed = true
	delete(s.sessions, sess)
}

// begin runs the common part of a call: hook, count, session state, injected
// error and latency. sess is nil for Dial. For ReadAt, it counts an open
// handle on success; the caller releases it.
func (s *Share) begin(ctx context.Context, op Op, p string, sess *Session) error {
	s.mu.Lock()
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		hook(op, p)
	}

	s.mu.Lock()
	s.calls[op]++
	latency := s.latency
	err := s.precheck(ctx, op, p, sess)
	if err == nil && op == OpReadAt {
		s.open++
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if latency > 0 {
		t := time.NewTimer(latency)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	if err != nil && op == OpReadAt {
		s.mu.Lock()
		s.open--
		s.mu.Unlock()
	}
	return err
}

// precheck returns the error a call fails with before doing any work. s.mu
// must be held.
func (s *Share) precheck(ctx context.Context, op Op, p string, sess *Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sess != nil {
		if sess.closed {
			return client.ErrClosed
		}
		if s.sessions[sess] {
			return &os.PathError{Op: op.String(), Path: p, Err: fmt.Errorf("fake: session dropped: %w", ErrTransient)}
		}
	}
	for _, k := range []errKey{{op: op, path: p}, {op: op, any: true}} {
		if queue := s.errs[k]; len(queue) > 0 {
			s.errs[k] = queue[1:]
			return queue[0]
		}
	}
	return nil
}

// file returns the file at p, creating it with a new FileID if needed. s.mu
// must be held.
func (s *Share) file(p string) *file {
	if f, ok := s.files[p]; ok {
		return f
	}
	if s.isDir(p) {
		panic(fmt.Sprintf("fake: %q is a directory", p))
	}
	// Every file gets its own creation time, even when the clock has not
	// moved since the previous one: tests that reuse a FileID rely on it.
	now := time.Now().Round(0)
	if !now.After(s.created) {
		now = s.created.Add(time.Microsecond)
	}
	s.created = now
	f := &file{id: s.nextID, ctime: now, mtime: now}
	s.nextID++
	s.files[p] = f
	s.addDirs(path.Dir(p))
	return f
}

// addDirs records dir and its parents. s.mu must be held.
func (s *Share) addDirs(dir string) {
	for dir = dirKey(dir); dir != ""; dir = dirKey(path.Dir(dir)) {
		s.dirs[dir] = true
	}
}

// isDir reports whether p is the root or a directory. s.mu must be held.
func (s *Share) isDir(p string) bool {
	return p == "" || s.dirs[p]
}

// dirKey maps path.Dir's "." for the root to "".
func dirKey(dir string) string {
	if dir == "." {
		return ""
	}
	return dir
}

// dirOf is the value path.Dir returns for the children of dir.
func dirOf(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

func mustClean(p string) string {
	clean, err := client.CleanPath(p)
	if err != nil {
		panic(err)
	}
	return clean
}
