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
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Equal(t, "auth failed for password ********: "+status(statusLogonFailure).Error(), err.Error())
	assert.Equal(t, ErrAuth, Classify(err), "the classification survives the redaction")
	assert.Nil(t, errors.Unwrap(err), "the original error, which holds the password, is dropped")
	for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
		assert.NotContains(t, fmt.Sprintf(verb, err), testPassword, verb)
	}
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
