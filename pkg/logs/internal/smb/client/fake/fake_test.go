// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package fake

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
)

var ctx = context.Background()

func names(entries []client.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func entry(t *testing.T, sess *Session, dir, name string) client.Entry {
	t.Helper()
	entries, err := sess.ListDir(ctx, dir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("%s not listed in %q: %v", name, dir, names(entries))
	return client.Entry{}
}

func TestWriteAppendAndRead(t *testing.T) {
	s := New()
	sess := s.NewSession()
	id := s.Write("app/a.log", []byte("hello "))
	assert.Equal(t, id, s.Append("app/a.log", []byte("world\n")), "appending keeps the FileID")

	res, err := sess.ReadAt(ctx, "app/a.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, client.ReadResult{FileID: id, Size: 12, Data: []byte("hello world\n")}, res)

	res, err = sess.ReadAt(ctx, "/app/a.log", 6, 3)
	require.NoError(t, err)
	assert.Equal(t, "wor", string(res.Data), "max bounds the read; a leading '/' is accepted")

	for _, off := range []int64{12, 50} {
		res, err = sess.ReadAt(ctx, "app/a.log", off, 100)
		require.NoError(t, err)
		assert.Equal(t, client.ReadResult{FileID: id, Size: 12}, res, "reading at or past the end is empty, not an error")
	}
	res, err = sess.ReadAt(ctx, "app/a.log", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, client.ReadResult{FileID: id, Size: 12}, res)

	res, err = sess.ReadAt(ctx, "app/a.log", 0, 5)
	require.NoError(t, err)
	res.Data[0] = 'X'
	again, err := sess.ReadAt(ctx, "app/a.log", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(again.Data), "returned data is a copy")

	s.Write("app/a.log", []byte("new"))
	got, ok := s.Stat("app/a.log")
	require.True(t, ok)
	assert.Equal(t, id, got.FileID, "Write replaces the content in place")
	assert.Equal(t, int64(3), got.Size)
}

func TestListDir(t *testing.T) {
	s := New()
	sess := s.NewSession()
	idB := s.Write("app/b.log", []byte("bb"))
	s.Write("app/a.log", []byte("a"))
	s.Write("app/old/c.log", []byte("c"))
	s.Write("root.log", nil)
	s.Mkdir("empty/deep")

	entries, err := sess.ListDir(ctx, "app")
	require.NoError(t, err)
	assert.Equal(t, []string{"a.log", "b.log", "old"}, names(entries), "sorted by name, subdirectories included")
	assert.Equal(t, client.Entry{Name: "b.log", Size: 2, FileID: idB, ModTime: entries[1].ModTime, CreationTime: entries[1].CreationTime}, entries[1])
	assert.True(t, entries[2].IsDir)

	entries, err = sess.ListDir(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"app", "empty", "root.log"}, names(entries))

	entries, err = sess.ListDir(ctx, "empty")
	require.NoError(t, err)
	assert.Equal(t, []string{"deep"}, names(entries))

	require.NoError(t, s.Delete("app/old/c.log"))
	entries, err = sess.ListDir(ctx, "app/old")
	require.NoError(t, err)
	assert.Empty(t, entries, "a directory outlives its last file")

	_, err = sess.ListDir(ctx, "nope")
	assert.Equal(t, client.ErrNotFound, client.Classify(err))
	_, err = sess.ListDir(ctx, "root.log")
	assert.Equal(t, client.ErrOther, client.Classify(err))
	_, err = sess.ListDir(ctx, "../x")
	assert.ErrorIs(t, err, os.ErrInvalid)
}

func TestRotations(t *testing.T) {
	s := New()
	sess := s.NewSession()
	id := s.Write("app.log", []byte("first generation\n"))

	// rename + create
	require.NoError(t, s.Rename("app.log", "app.log.1"))
	newID := s.Recreate("app.log", 0)
	assert.NotEqual(t, id, newID)
	assert.Equal(t, id, entry(t, sess, "", "app.log.1").FileID, "a rename keeps the FileID")
	assert.Equal(t, newID, entry(t, sess, "", "app.log").FileID)
	res, err := sess.ReadAt(ctx, "app.log.1", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, "first generation\n", string(res.Data))

	// a rename replaces the target, like a second rotation
	s.Append("app.log", []byte("second\n"))
	require.NoError(t, s.Rename("app.log", "app.log.1"))
	assert.Equal(t, newID, entry(t, sess, "", "app.log.1").FileID)

	// copytruncate keeps the FileID and shrinks the file
	s.Write("ct.log", []byte("0123456789"))
	ctID, _ := s.Stat("ct.log")
	require.NoError(t, s.Truncate("ct.log", 0))
	res, err = sess.ReadAt(ctx, "ct.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, client.ReadResult{FileID: ctID.FileID, Size: 0}, res)
	require.NoError(t, s.Truncate("ct.log", 3))
	res, err = sess.ReadAt(ctx, "ct.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 0, 0}, res.Data)

	// delete + recreate, with a reused FileID (Samba inodes)
	reused := s.Write("reuse.log", []byte("a long first file\n"))
	require.NoError(t, s.Delete("reuse.log"))
	_, err = sess.ReadAt(ctx, "reuse.log", 0, 1)
	assert.Equal(t, client.ErrNotFound, client.Classify(err))
	assert.Equal(t, reused, s.Recreate("reuse.log", reused))
	s.Append("reuse.log", []byte("short\n"))
	res, err = sess.ReadAt(ctx, "reuse.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, client.ReadResult{FileID: reused, Size: 6, Data: []byte("short\n")}, res)

	// writer-side errors
	assert.Equal(t, client.ErrNotFound, client.Classify(s.Rename("nope", "x")))
	assert.Equal(t, client.ErrNotFound, client.Classify(s.Truncate("nope", 0)))
	assert.Equal(t, client.ErrNotFound, client.Classify(s.Delete("nope")))
}

func TestStaleListing(t *testing.T) {
	s := New()
	sess := s.NewSession()
	s.Write("a.log", []byte("1234"))

	s.SetStaleListing(true)
	s.Append("a.log", []byte("5678"))
	s.Write("new.log", []byte("abc"))
	assert.Equal(t, int64(4), entry(t, sess, "", "a.log").Size, "the listing keeps the old size")
	assert.Equal(t, int64(0), entry(t, sess, "", "new.log").Size, "a file created while stale lists as empty")
	res, err := sess.ReadAt(ctx, "a.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(8), res.Size, "ReadAt always sees the real size")

	s.SetStaleListing(false)
	assert.Equal(t, int64(8), entry(t, sess, "", "a.log").Size)
	assert.Equal(t, int64(3), entry(t, sess, "", "new.log").Size)
}

func TestFileIDReporting(t *testing.T) {
	s := New()
	sess := s.NewSession()
	id := s.Write("a.log", []byte("x"))

	s.SetListingFileIDs(false)
	assert.Zero(t, entry(t, sess, "", "a.log").FileID)
	res, err := sess.ReadAt(ctx, "a.log", 0, 1)
	require.NoError(t, err)
	assert.Equal(t, id, res.FileID)

	s.SetListingFileIDs(true)
	s.SetReadFileIDs(false)
	assert.Equal(t, id, entry(t, sess, "", "a.log").FileID)
	res, err = sess.ReadAt(ctx, "a.log", 0, 1)
	require.NoError(t, err)
	assert.Zero(t, res.FileID)
}

func TestInjectedErrors(t *testing.T) {
	s := New()
	sess := s.NewSession()
	s.Write("a.log", []byte("x"))
	s.Write("b.log", []byte("y"))
	boom := errors.New("boom")

	s.FailNext(OpReadAt, ErrSharing, nil, boom)
	s.FailNextPath(OpReadAt, "b.log", ErrNotFound)

	_, err := sess.ReadAt(ctx, "b.log", 0, 1)
	assert.ErrorIs(t, err, ErrNotFound, "a path-specific error goes first")
	_, err = sess.ReadAt(ctx, "b.log", 0, 1)
	assert.ErrorIs(t, err, ErrSharing)
	assert.Equal(t, client.ErrSharing, client.Classify(err))
	_, err = sess.ReadAt(ctx, "a.log", 0, 1)
	assert.NoError(t, err, "a nil entry lets the call through")
	_, err = sess.ReadAt(ctx, "a.log", 0, 1)
	assert.ErrorIs(t, err, boom)
	_, err = sess.ReadAt(ctx, "a.log", 0, 1)
	assert.NoError(t, err, "the queue is empty")

	s.FailNextPath(OpListDir, "", ErrAccessDenied)
	_, err = sess.ListDir(ctx, "")
	assert.Equal(t, client.ErrAuth, client.Classify(err))
	_, err = sess.ListDir(ctx, "")
	assert.NoError(t, err)
	assert.Equal(t, client.ErrTransient, client.Classify(ErrOverloaded))

	assert.Equal(t, 5, s.Calls(OpReadAt), "failed calls count too")
	assert.Equal(t, 2, s.Calls(OpListDir))
}

func TestSessions(t *testing.T) {
	s := New()
	s.Write("a.log", []byte("x"))

	c1, err := s.Dial(ctx, client.Config{})
	require.NoError(t, err)
	assert.Equal(t, 1, s.LiveSessions())

	s.FailNext(OpDial, ErrAuth)
	_, err = s.Dial(ctx, client.Config{})
	assert.Equal(t, client.ErrAuth, client.Classify(err))
	assert.Equal(t, 2, s.Calls(OpDial))

	s.DropSessions()
	_, err = c1.ReadAt(ctx, "a.log", 0, 1)
	assert.Equal(t, client.ErrTransient, client.Classify(err))
	_, err = c1.ListDir(ctx, "")
	assert.Equal(t, client.ErrTransient, client.Classify(err))

	c2, err := s.Dial(ctx, client.Config{})
	require.NoError(t, err)
	_, err = c2.ReadAt(ctx, "a.log", 0, 1)
	assert.NoError(t, err, "a session dialed after the drop works")
	assert.Equal(t, 2, s.LiveSessions())

	require.NoError(t, c1.Close())
	require.NoError(t, c1.Close())
	assert.Equal(t, 1, s.LiveSessions())
	_, err = c1.ReadAt(ctx, "a.log", 0, 1)
	assert.ErrorIs(t, err, client.ErrClosed)
}

func TestHookAndOpenHandles(t *testing.T) {
	s := New()
	sess := s.NewSession()
	s.Write("app.log", []byte("old"))

	// The hook runs before the call and may rotate the file under it.
	s.SetHook(func(op Op, p string) {
		if op == OpReadAt && p == "app.log" {
			s.SetHook(nil)
			require.NoError(t, s.Rename("app.log", "app.log.1"))
			s.Write("app.log", []byte("new!"))
		}
	})
	res, err := sess.ReadAt(ctx, "app.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, "new!", string(res.Data))

	// A handle is open only while ReadAt runs. The read lasts until it is
	// canceled, so the open handle is seen however the goroutines are
	// scheduled.
	s.SetLatency(time.Hour)
	rctx, cancelRead := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := sess.ReadAt(rctx, "app.log", 0, 1)
		done <- err
	}()
	require.Eventually(t, func() bool { return s.OpenHandles() == 1 }, 5*time.Second, time.Millisecond)
	cancelRead()
	assert.ErrorIs(t, <-done, context.Canceled, "latency gives way to the context")
	assert.Zero(t, s.OpenHandles(), "a canceled read releases its handle")

	tctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = sess.ReadAt(tctx, "app.log", 0, 1)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Zero(t, s.OpenHandles())
}

func TestLogoffLatencyAndAbort(t *testing.T) {
	s := New()
	s.SetLogoffLatency(time.Hour) // a server that never answers LOGOFF
	sess := s.NewSession()

	closed := make(chan error, 1)
	go func() { closed <- sess.Close() }()
	require.Eventually(t, func() bool { return s.Calls(OpLogoff) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, 1, s.LiveSessions(), "a session logging off is still live")
	select {
	case <-closed:
		require.FailNow(t, "Close did not wait for the logoff")
	default:
	}

	require.NoError(t, sess.Abort())
	assert.NoError(t, <-closed, "Abort cuts the logoff short")
	assert.Zero(t, s.LiveSessions())
	require.NoError(t, sess.Abort(), "Abort is idempotent")
	require.NoError(t, sess.Close())
	assert.Equal(t, 1, s.Calls(OpLogoff), "a closed session does not log off again")

	other := s.NewSession()
	require.NoError(t, other.Abort())
	assert.Zero(t, s.LiveSessions())
	_, err := other.ListDir(ctx, "")
	assert.ErrorIs(t, err, client.ErrClosed)
}

func TestReadAtRejectsBadArguments(t *testing.T) {
	s := New()
	sess := s.NewSession()
	s.Write("a.log", []byte("x"))
	s.Mkdir("dir")

	for _, tc := range []struct {
		p   string
		off int64
		n   int
	}{{"", 0, 1}, {"a.log", -1, 1}, {"a.log", 0, -1}, {"../a.log", 0, 1}} {
		_, err := sess.ReadAt(ctx, tc.p, tc.off, tc.n)
		assert.ErrorIs(t, err, os.ErrInvalid, "%+v", tc)
	}
	_, err := sess.ReadAt(ctx, "dir", 0, 1)
	require.Error(t, err)
	assert.Equal(t, client.ErrOther, client.Classify(err))
	assert.Zero(t, s.OpenHandles())
}
