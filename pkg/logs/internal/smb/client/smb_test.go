// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	smb2 "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
)

const testPassword = "hunter2-Sup3rS3cret"

// fakeFile serves readChunk calls for readLoop tests and records them.
type fakeFile struct {
	id    uint64
	data  []byte
	calls []string
	// at, when set, is called before each chunk and may change the file or
	// fail the call.
	at func(call int) error
}

func (f *fakeFile) readChunk(off int64, n int) (chunk, error) {
	f.calls = append(f.calls, fmt.Sprintf("%d+%d", off, n))
	if f.at != nil {
		if err := f.at(len(f.calls)); err != nil {
			return chunk{}, err
		}
	}
	ch := chunk{fileID: f.id, size: int64(len(f.data))}
	if off < ch.size {
		ch.data = bytes.Clone(f.data[off:min(ch.size, off+int64(n))])
	}
	return ch, nil
}

func TestReadLoop(t *testing.T) {
	data := []byte("0123456789abcdefghij") // 20 bytes

	t.Run("max 0 only reports identity and size", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data}
		res, err := readLoop(5, 0, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, ReadResult{FileID: 7, Size: 20}, res)
		assert.Equal(t, []string{"5+0"}, f.calls)
	})

	t.Run("offset at or past the end is an empty read", func(t *testing.T) {
		for _, off := range []int64{20, 25} {
			f := &fakeFile{id: 7, data: data}
			res, err := readLoop(off, 8, 4, f.readChunk)
			require.NoError(t, err)
			assert.Equal(t, ReadResult{FileID: 7, Size: 20}, res)
			assert.Len(t, f.calls, 1)
		}
	})

	t.Run("one chunk when it covers max", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data}
		res, err := readLoop(2, 3, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, "234", string(res.Data))
		assert.Equal(t, []string{"2+3"}, f.calls)
	})

	t.Run("chunks until max", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data}
		res, err := readLoop(1, 10, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, "123456789a", string(res.Data))
		assert.Equal(t, []string{"1+4", "5+4", "9+2"}, f.calls)
	})

	t.Run("chunks until the end of the file", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data}
		res, err := readLoop(10, 100, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, "abcdefghij", string(res.Data))
		assert.Equal(t, int64(20), res.Size)
		assert.Equal(t, []string{"10+4", "14+4", "18+4"}, f.calls, "stops once it reached the size seen at open")
	})

	t.Run("an exact end of file needs no extra round trip", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data}
		_, err := readLoop(12, 100, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, []string{"12+4", "16+4"}, f.calls)
	})

	t.Run("short read stops", func(t *testing.T) {
		calls := 0
		res, err := readLoop(0, 12, 4, func(off int64, n int) (chunk, error) {
			calls++
			if off == 4 {
				return chunk{fileID: 7, size: 20, data: []byte("45")}, nil // server returned less than asked
			}
			return chunk{fileID: 7, size: 20, data: data[off : off+int64(n)]}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "012345", string(res.Data))
		assert.Equal(t, 2, calls)
	})

	t.Run("oversized server response is truncated", func(t *testing.T) {
		res, err := readLoop(0, 3, 4, func(_ int64, _ int) (chunk, error) {
			return chunk{fileID: 7, size: 20, data: data[:10]}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "012", string(res.Data))
	})

	t.Run("rotation between chunks keeps the first file's bytes only", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data}
		f.at = func(call int) error {
			if call == 2 {
				f.id, f.data = 8, []byte("NEW FILE CONTENT, NOT TO BE MIXED")
			}
			return nil
		}
		res, err := readLoop(0, 12, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, ReadResult{FileID: 7, Size: 20, Data: []byte("0123")}, res)
	})

	t.Run("a new creation time between chunks keeps the first file's bytes only", func(t *testing.T) {
		created := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
		calls := 0
		res, err := readLoop(0, 12, 4, func(off int64, n int) (chunk, error) {
			calls++
			ch := chunk{fileID: 7, created: created, size: 20, data: data[off : off+int64(n)]}
			if calls == 2 {
				// Deleted and recreated under the same FileId.
				ch.created = created.Add(time.Second)
			}
			return ch, nil
		})
		require.NoError(t, err)
		assert.Equal(t, ReadResult{FileID: 7, CreationTime: created, Size: 20, Data: []byte("0123")}, res)
	})

	t.Run("truncation between chunks stops without FileIDs", func(t *testing.T) {
		f := &fakeFile{data: data}
		f.at = func(call int) error {
			if call == 2 {
				f.data = []byte("xy")
			}
			return nil
		}
		res, err := readLoop(0, 12, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, ReadResult{Size: 20, Data: []byte("0123")}, res)
	})

	t.Run("error on the first chunk is returned", func(t *testing.T) {
		boom := errors.New("boom")
		f := &fakeFile{id: 7, data: data, at: func(int) error { return boom }}
		res, err := readLoop(0, 12, 4, f.readChunk)
		assert.ErrorIs(t, err, boom)
		assert.Equal(t, ReadResult{}, res)
	})

	t.Run("error on a later chunk keeps the bytes already read", func(t *testing.T) {
		f := &fakeFile{id: 7, data: data, at: func(call int) error {
			if call == 3 {
				return errors.New("boom")
			}
			return nil
		}}
		res, err := readLoop(0, 12, 4, f.readChunk)
		require.NoError(t, err)
		assert.Equal(t, "01234567", string(res.Data))
	})
}

func TestReadRangeOverlapsOneByte(t *testing.T) {
	file := []byte("0123456789") // 10 bytes
	// serve answers a READ the way a server does: STATUS_END_OF_FILE at or
	// past the end, a short read when the range crosses it.
	serve := func(start int64, length int) ([]byte, bool) {
		if start >= int64(len(file)) {
			return nil, false
		}
		return file[start:min(int64(len(file)), start+int64(length))], true
	}

	for _, tc := range []struct {
		off    int64
		n      int
		start  int64
		length int
		want   string
		eof    bool
	}{
		{off: 0, n: 4, start: 0, length: 4, want: "0123"},
		{off: 3, n: 4, start: 2, length: 5, want: "3456"},
		{off: 8, n: 4, start: 7, length: 5, want: "89"},
		{off: 10, n: 4, start: 9, length: 5, want: ""},   // nothing new: still a successful READ
		{off: 11, n: 4, start: 10, length: 5, eof: true}, // truncated below off
	} {
		start, length := readRange(tc.off, tc.n)
		assert.Equal(t, tc.start, start, "%+v", tc)
		assert.Equal(t, tc.length, length, "%+v", tc)

		data, ok := serve(start, length)
		assert.Equal(t, tc.eof, !ok, "%+v", tc)
		if ok {
			got := readData(tc.off, tc.n, data)
			assert.Equal(t, tc.want, string(got), "%+v", tc)
			assert.LessOrEqual(t, len(got), tc.n)
		}
	}

	assert.Nil(t, readData(5, 4, []byte("5")), "an empty result is nil, like a read of nothing")
	assert.Nil(t, readData(5, 4, nil), "a server returning no data at all")
	assert.Equal(t, "678", string(readData(6, 3, []byte("56789"))), "an oversized response is cut to n")

	// The overlap byte must not push a full chunk over one credit (64 KiB).
	_, length := readRange(1, readChunkSize-1)
	assert.LessOrEqual(t, length, readChunkSize)
}

func TestReadHitEndOfFile(t *testing.T) {
	eof := &protocol.CompoundResponseError{Errors: []error{nil, status(statusEndOfFile), nil}}
	assert.True(t, readHitEndOfFile(eof))

	createFailed := &protocol.CompoundResponseError{Errors: []error{status(statusEndOfFile), status(statusEndOfFile), nil}}
	assert.False(t, readHitEndOfFile(createFailed), "the CREATE itself failed")

	otherRead := &protocol.CompoundResponseError{Errors: []error{nil, status(statusFileLockConflict), nil}}
	assert.False(t, readHitEndOfFile(otherRead))

	assert.False(t, readHitEndOfFile(status(statusEndOfFile)), "not a compound response")
	assert.False(t, readHitEndOfFile(errors.New("boom")))
}

func TestWatchdog(t *testing.T) {
	const grace = 20 * time.Millisecond

	t.Run("aborts an operation that outlives its context", func(t *testing.T) {
		aborted := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		done := watchdog(ctx, grace, func() { close(aborted) })
		defer done()
		cancel()
		select {
		case <-aborted:
		case <-time.After(5 * time.Second):
			t.Fatal("abort was not called")
		}
	})

	t.Run("does nothing when the operation returns first", func(t *testing.T) {
		var aborts atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := watchdog(ctx, grace, func() { aborts.Add(1) })
		done()
		cancel()
		time.Sleep(3 * grace)
		assert.Zero(t, aborts.Load())
	})

	t.Run("does nothing when the operation returns within the grace period", func(t *testing.T) {
		var aborts atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := watchdog(ctx, time.Hour, func() { aborts.Add(1) })
		cancel()
		time.Sleep(grace) // let the context callback start the grace timer
		done()
		assert.Zero(t, aborts.Load())
	})
}

func TestRedactErr(t *testing.T) {
	assert.NoError(t, redactErr(nil, testPassword))

	clean := errors.New("connection refused")
	assert.Same(t, clean, redactErr(clean, testPassword), "an error without the secret is returned as is")
	assert.Same(t, clean, redactErr(clean, ""), "nothing to redact without a password")

	leaky := fmt.Errorf("auth failed for password %s: %w", testPassword, status(statusLogonFailure))
	err := redactErr(leaky, testPassword)
	assert.Equal(t, redactedMessage, err.Error(), "the whole message goes: no substring of it is kept for a guess to probe")
	assert.Equal(t, ErrAuth, Classify(err), "the classification survives the redaction")
	code, ok := statusCode(err)
	assert.True(t, ok)
	assert.Equal(t, uint32(statusLogonFailure), code, "so does the status code")
	assert.Nil(t, errors.Unwrap(err), "the original error, which holds the password, is dropped")
	for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
		assert.NotContains(t, fmt.Sprintf(verb, err), testPassword, verb)
	}

	// The sentinels of this package survive too, and a password of one character
	// is looked for like any other.
	guest := redactErr(fmt.Errorf("%s: %w", "x", errGuestSession), "x")
	assert.Equal(t, redactedMessage, guest.Error())
	assert.ErrorIs(t, guest, errGuestSession)
	assert.Equal(t, ErrAuth, Classify(guest))
	assert.NotErrorIs(t, guest, ErrClosed)
}

func TestRedactionKeepsAgentText(t *testing.T) {
	cfg := Config{Host: "files.example.com", Share: "logs", Username: "u", Password: "encrypt the session", RequireEncryption: true}.withDefaults()

	// The encryption check: the error is the Agent's own text, which holds the
	// password.
	refusal := checkEncryption(encrypted(false), true)
	require.ErrorIs(t, refusal, ErrNotEncrypted)
	require.Contains(t, refusal.Error(), cfg.Password)
	err := checked(fmt.Errorf("smb: connect to %s: %w", cfg.target(), refusal))
	assert.Same(t, err, redactErr(err, cfg.Password), "text built by Dial is not searched again")
	assert.ErrorIs(t, err, ErrNotEncrypted)
	assert.Contains(t, err.Error(), "does not encrypt")
	assert.Equal(t, ErrAuth, Classify(err))

	// A library error that holds the password is redacted before the Agent's
	// text is added, and the guest sentinel survives.
	guestLib := fmt.Errorf("%s: %w", cfg.Password, &protocol.InvalidResponseError{Message: "guest account doesn't support signing"})
	guestErr := dialError(guestLib, cfg, cfg.target(), true)
	assert.NotContains(t, guestErr.Error(), cfg.Password)
	assert.Contains(t, guestErr.Error(), "check the username and password")
	assert.ErrorIs(t, guestErr, errGuestSession)
	assert.Equal(t, ErrAuth, Classify(guestErr), "a guest session needs user action, not a retry")
	assert.Same(t, guestErr, redactErr(guestErr, cfg.Password))
}

func TestSMBClientRejectsBadArguments(t *testing.T) {
	c := &smbClient{target: "smb://h/s", opTimeout: time.Second}
	ctx := context.Background()

	for _, tc := range []struct {
		path     string
		off      int64
		max      int
		contains string
	}{
		{"", 0, 1, "invalid"},
		{"/", 0, 1, "invalid"},
		{"../etc/passwd", 0, 1, "invalid"},
		{"a.log", -1, 1, "invalid"},
		{"a.log", 0, -1, "invalid"},
	} {
		_, err := c.ReadAt(ctx, tc.path, tc.off, tc.max)
		assert.ErrorIs(t, err, os.ErrInvalid, "%+v", tc)
	}
	_, err := c.ListDir(ctx, "../x")
	assert.ErrorIs(t, err, os.ErrInvalid)

	c.closed.Store(true)
	_, err = c.ReadAt(ctx, "a.log", 0, 1)
	assert.ErrorIs(t, err, ErrClosed)
	_, err = c.ListDir(ctx, "")
	assert.ErrorIs(t, err, ErrClosed)
}

func TestDialerRequiresSigning(t *testing.T) {
	cfg := Config{Host: "h", Share: "s", Username: "u", Password: testPassword, Domain: "CORP"}.withDefaults()
	d := newDialer(cfg)
	assert.True(t, d.RequireMessageSigning, "unsigned sessions, including guest and anonymous ones, are refused")
	assert.True(t, d.DisableAAPLExtension)
}

func TestDialerOffersOnlySMB3ByDefault(t *testing.T) {
	cfg := Config{Host: "h", Share: "s", Username: "u", Password: testPassword}.withDefaults()
	assert.Equal(t, []smb2.Dialect{smb2.SMB311, smb2.SMB302, smb2.SMB300}, newDialer(cfg).SpecifiedDialects,
		"SMB 2.x cannot be encrypted and its negotiation is not authenticated: it is not offered unless the source opts in")

	cfg.AllowSMB2 = true
	assert.Empty(t, newDialer(cfg).SpecifiedDialects, "allow_smb2 leaves the library's list, SMB 3.1.1 down to 2.0.2")
}

type encrypted bool

func (e encrypted) Encrypted() bool { return bool(e) }

func TestRequireEncryptionFailsClosed(t *testing.T) {
	assert.NoError(t, checkEncryption(encrypted(true), true))
	assert.NoError(t, checkEncryption(encrypted(true), false))
	assert.NoError(t, checkEncryption(encrypted(false), false), "encryption is opt-in")

	err := checkEncryption(encrypted(false), true)
	require.ErrorIs(t, err, ErrNotEncrypted)
	assert.Equal(t, ErrAuth, Classify(err), "it needs a change of the server or of the source, so it backs off like an auth failure")
	assert.Equal(t, ErrAuth, Classify(fmt.Errorf("smb: connect to smb://h/s: %w", err)))

	assert.ErrorContains(t, Config{Host: "h", Share: "s", AllowSMB2: true, RequireEncryption: true}.validate(), "cannot be combined with allow_smb2")
	assert.NoError(t, Config{Host: "h", Share: "s", RequireEncryption: true}.validate())
}

func TestNegotiateHintForServersWithoutSMB3(t *testing.T) {
	notSupported := fmt.Errorf("smb: connect to smb://h/s: %w", status(statusNotSupported))
	cfg := Config{Host: "h", Share: "s"}

	err := negotiateHint(notSupported, cfg)
	assert.ErrorIs(t, err, notSupported)
	assert.ErrorContains(t, err, "allow_smb2: true")

	cfg.AllowSMB2 = true
	assert.Equal(t, notSupported, negotiateHint(notSupported, cfg), "no hint when SMB 2 is already allowed")
	cfg.AllowSMB2 = false
	other := status(statusAccessDenied)
	assert.Equal(t, other, negotiateHint(other, cfg))
	assert.Equal(t, io.EOF, negotiateHint(io.EOF, cfg))
}

// fakeInfo is a directory entry as the library reports it.
type fakeInfo struct {
	name string
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 1 }
func (f fakeInfo) Mode() os.FileMode  { return 0 }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return nil }

// pages serves a directory of total entries the way File.Readdir does: full
// batches, then a short one (or none, with io.EOF) at the end.
func pages(total int, reads *int) func(n int) ([]os.FileInfo, error) {
	next := 0
	return func(n int) ([]os.FileInfo, error) {
		*reads++
		var infos []os.FileInfo
		for len(infos) < n && next < total {
			infos = append(infos, fakeInfo{name: fmt.Sprintf("f%07d.log", next)})
			next++
		}
		if len(infos) == 0 {
			return nil, io.EOF
		}
		return infos, nil
	}
}

func TestReadEntries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		total int
		reads int // batch reads the listing takes
	}{
		{"empty directory", 0, 1},
		{"less than a batch", 10, 1},
		{"one full batch, then the end", 1024, 2},
		{"several batches", 2500, 3},
		{"exactly the limit", 5000, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reads int
			entries, err := readEntries(pages(tc.total, &reads), 1024, 5000, maxListBytes)
			require.NoError(t, err)
			assert.Len(t, entries, tc.total)
			assert.Equal(t, tc.reads, reads)
		})
	}
}

// TestReadEntriesRefusesWhatTheLimitCannotHold covers a directory over the
// limit, and a server that never stops sending entries: the listing stops
// after the batch that crosses the limit, and returns an error, never part of
// the directory.
func TestReadEntriesRefusesWhatTheLimitCannotHold(t *testing.T) {
	var reads int
	entries, err := readEntries(pages(5001, &reads), 1024, 5000, maxListBytes)
	require.ErrorIs(t, err, ErrTooManyEntries)
	assert.Nil(t, entries)
	assert.Equal(t, 5, reads, "the batch that crossed the limit was the last one read")
	assert.Equal(t, ErrTooLarge, Classify(err))

	reads = 0
	_, err = readEntries(pages(1<<30, &reads), 1024, maxListEntries, maxListBytes)
	require.ErrorIs(t, err, ErrTooManyEntries)
	assert.LessOrEqual(t, reads, maxListEntries/listBatch+1, "an endless listing stops at the limit")
}

// namedPages serves a directory that never ends, whose entries are named by
// name(i), the way File.Readdir does.
func namedPages(name func(i int) string, reads *int) func(n int) ([]os.FileInfo, error) {
	next := 0
	return func(n int) ([]os.FileInfo, error) {
		*reads++
		infos := make([]os.FileInfo, 0, n)
		for range n {
			infos = append(infos, fakeInfo{name: name(next)})
			next++
		}
		return infos, nil
	}
}

// TestReadEntriesStopsAtTheByteBudget: a count of entries does not bound the
// memory of a listing whose names are long, so the entries have a budget in
// bytes too, and the listing stops once a batch crosses it. With the limits of
// ListDir, a server that sends names of the longest allowed length, in
// multibyte characters, takes the budget long before the count.
func TestReadEntriesStopsAtTheByteBudget(t *testing.T) {
	cjk := strings.Repeat("日", maxNameUnits) // 255 UTF-16 code units, 765 bytes
	var reads int
	entries, err := readEntries(namedPages(func(int) string { return cjk }, &reads), listBatch, maxListEntries, maxListBytes)
	require.ErrorIs(t, err, ErrListingTooLarge)
	assert.Nil(t, entries)
	perEntry := len(cjk) + entryOverhead
	assert.Equal(t, maxListBytes/perEntry/listBatch+1, reads, "the batch that crossed the budget was the last one read")
	assert.Less(t, reads*listBatch, maxListEntries, "the budget, not the count, stopped it")
	assert.Equal(t, ErrTooLarge, Classify(err))

	// Under the budget, the same entries are listed.
	reads = 0
	entries, err = readEntries(pages(3000, &reads), listBatch, maxListEntries, 3000*(len("f0000000.log")+entryOverhead))
	require.NoError(t, err)
	assert.Len(t, entries, 3000)
}

// TestReadEntriesRefusesLongNames: a name longer than 255 UTF-16 code units, the
// longest a file name can be, refuses the whole listing.
func TestReadEntriesRefusesLongNames(t *testing.T) {
	for name, tc := range map[string]struct {
		name string
		ok   bool
	}{
		"255 ASCII characters":           {strings.Repeat("a", 255), true},
		"256 ASCII characters":           {strings.Repeat("a", 256), false},
		"255 characters of 3 bytes":      {strings.Repeat("日", 255), true},
		"256 characters of 3 bytes":      {strings.Repeat("日", 256), false},
		"127 astral characters (254)":    {strings.Repeat("😀", 127), true},
		"128 astral characters (256)":    {strings.Repeat("😀", 128), false},
		"a name the size of a page":      {strings.Repeat("a", 64*1024), false},
		"an empty name stays an entry":   {"", true},
		"a name with invalid UTF-8 byte": {strings.Repeat("\xff", 256), false},
	} {
		t.Run(name, func(t *testing.T) {
			entries, err := readEntries(func(int) ([]os.FileInfo, error) {
				return []os.FileInfo{fakeInfo{name: "ok.log"}, fakeInfo{name: tc.name}}, io.EOF
			}, listBatch, maxListEntries, maxListBytes)
			if tc.ok {
				require.NoError(t, err)
				assert.Len(t, entries, 2)
				return
			}
			require.ErrorIs(t, err, ErrNameTooLong)
			assert.Nil(t, entries, "no part of the listing is returned")
			assert.Equal(t, ErrTooLarge, Classify(err))
		})
	}
}

func TestListingRefusals(t *testing.T) {
	err := ListingTooLarge("app/logs")
	assert.Equal(t, ErrTooLarge, Classify(err))
	assert.Equal(t, ErrTooLarge, Classify(fmt.Errorf("list: %w", err)))
	assert.Contains(t, err.Error(), `"app/logs"`)
	assert.Contains(t, err.Error(), "64 MiB")

	err = NameTooLong("app/logs")
	assert.Equal(t, ErrTooLarge, Classify(err))
	assert.Contains(t, err.Error(), `"app/logs"`)
	assert.Contains(t, err.Error(), "255 characters")

	// One batch past the limits is what a server can make the client hold:
	// listBatch entries of a directory page, 64 KiB, each, and the entries decoded
	// from them, about 6 MiB in all.
	assert.LessOrEqual(t, listBatch*64*1024, 4<<20)
	assert.Equal(t, 6<<20, listBatch*64*1024+2<<20)
}

func TestReadEntriesErrors(t *testing.T) {
	boom := errors.New("boom")
	_, err := readEntries(func(int) ([]os.FileInfo, error) { return []os.FileInfo{fakeInfo{name: "a"}}, boom }, 1024, 5000, maxListBytes)
	assert.ErrorIs(t, err, boom, "a failed read returns its error, not the entries read so far")
}

func TestTooManyEntriesError(t *testing.T) {
	err := TooManyEntries("app/logs")
	assert.Equal(t, ErrTooLarge, Classify(err))
	assert.Equal(t, ErrTooLarge, Classify(fmt.Errorf("list: %w", err)))
	assert.Contains(t, err.Error(), `"app/logs"`)
	assert.Contains(t, err.Error(), "100000 entries")
	assert.Equal(t, "too large", ErrTooLarge.String())
	assert.Equal(t, 100_000, maxListEntries)
}

func TestGuestSessionIsAnAuthError(t *testing.T) {
	for _, msg := range []string{"guest account doesn't support signing", "anonymous account doesn't support signing"} {
		// What the library returns for a SESSION_SETUP response flagged
		// IS_GUEST or IS_NULL when signing is required.
		libErr := &protocol.InvalidResponseError{Message: msg}
		require.True(t, isGuestSession(fmt.Errorf("dial: %w", libErr)), msg)
		err := fmt.Errorf("smb: connect to smb://h/s: %w", fmt.Errorf("%w: %w", errGuestSession, libErr))
		assert.Equal(t, ErrAuth, Classify(err), "a guest session needs user action, not a retry")
		assert.Contains(t, err.Error(), "check the username and password")
	}
	assert.False(t, isGuestSession(&protocol.InvalidResponseError{Message: "broken session setup response format"}))
	assert.False(t, isGuestSession(errors.New("guest account doesn't support signing")))
	assert.Equal(t, ErrTransient, Classify(&protocol.InvalidResponseError{Message: "broken"}))
}

func TestDialRejectsInvalidConfig(t *testing.T) {
	for _, cfg := range []Config{
		{Share: "s", Password: testPassword},
		{Host: "127.0.0.1", Password: testPassword},
		{Host: "127.0.0.1", Share: "a/b", Password: testPassword},
	} {
		c, err := Dial(context.Background(), cfg)
		require.Error(t, err)
		assert.Nil(t, c)
		assert.NotContains(t, err.Error(), testPassword)
	}
}

// localServer runs a TCP server on 127.0.0.1 that hands each connection to
// handle, and returns its port.
func localServer(t *testing.T, handle func(net.Conn)) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func testConfig(port int) Config {
	return Config{
		Host:        "127.0.0.1",
		Port:        port,
		Share:       "logs",
		Username:    "myacct",
		Password:    testPassword,
		Domain:      "WORKGROUP",
		DialTimeout: 500 * time.Millisecond,
	}
}

func TestDialFailuresAreTransientAndRedacted(t *testing.T) {
	closedPort := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := ln.Addr().(*net.TCPAddr).Port
		require.NoError(t, ln.Close())
		return port
	}

	for _, tc := range []struct {
		name string
		port func() int
	}{
		{"connection refused", closedPort},
		{"server closes the connection", func() int {
			return localServer(t, func(conn net.Conn) { conn.Close() })
		}},
		{"server never answers", func() int {
			stop := make(chan struct{})
			t.Cleanup(func() { close(stop) })
			return localServer(t, func(conn net.Conn) {
				defer conn.Close()
				<-stop
			})
		}},
		{"server answers garbage", func() int {
			return localServer(t, func(conn net.Conn) {
				defer conn.Close()
				_, _ = conn.Write([]byte{0, 0, 0, 8, 'n', 'o', 't', 's', 'm', 'b', '!', '!'})
				_, _ = conn.Read(make([]byte, 1024))
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(tc.port())
			start := time.Now()
			c, err := Dial(context.Background(), cfg)
			require.Error(t, err)
			assert.Nil(t, c)
			assert.Less(t, time.Since(start), 5*time.Second, "Dial is bounded by DialTimeout")
			assert.Equal(t, ErrTransient, Classify(err), "%v", err)
			assert.Contains(t, err.Error(), "smb://127.0.0.1:")
			assert.NotContains(t, err.Error(), testPassword)
			assert.NotContains(t, fmt.Sprintf("%#v", err), testPassword)
		})
	}
}

func TestDialHonorsCallerContext(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	port := localServer(t, func(conn net.Conn) {
		defer conn.Close()
		<-stop
	})
	cfg := testConfig(port)
	cfg.DialTimeout = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Dial(ctx, cfg)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, ErrTransient, Classify(err))
}

func TestListingCostCountsAsReadEntriesDoes(t *testing.T) {
	entries := []Entry{{Name: "a"}, {Name: "bcd"}}
	assert.Equal(t, 1+3+2*entryOverhead, ListingCost(entries))
	read := func(maxBytes int) ([]Entry, error) {
		done := false
		return readEntries(func(int) ([]os.FileInfo, error) {
			if done {
				return nil, io.EOF
			}
			done = true
			return []os.FileInfo{fakeInfo{name: "a"}, fakeInfo{name: "bcd"}}, nil
		}, 2, 100, maxBytes)
	}
	_, err := read(ListingCost(entries) - 1)
	assert.ErrorIs(t, err, ErrListingTooLarge, "ListDir refuses a listing just over the cost ListingCost gives it")
	got, err := read(ListingCost(entries))
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
