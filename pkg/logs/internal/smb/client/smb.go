// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	smb2 "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/auth"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

const (
	// readChunkSize is the largest READ sent in one request. The library
	// refuses servers whose MaxReadSize is below 64 KiB ([MS-SMB2] 3.2.5.2),
	// so every session accepts it and it costs a single credit. The library
	// keeps the negotiated maximum private, so a larger READ could fail with
	// STATUS_INVALID_PARAMETER on some servers.
	readChunkSize = 64 * 1024

	// abortGrace is how long an operation may keep running after its context
	// ended before the session is aborted. READ and QUERY_DIRECTORY return as
	// soon as the context ends, but CREATE and TREE_CONNECT wait for the
	// server's final response, which a dead server never sends.
	abortGrace = time.Second

	// readAccess opens a file for reading data and attributes only, so the
	// open never needs (or breaks) a writer's oplock or lease.
	readAccess = wire.FILE_READ_DATA | wire.FILE_READ_ATTRIBUTES | wire.SYNCHRONIZE
)

// smbClient is one authenticated session with one mounted share.
type smbClient struct {
	target    string // smb://host/share, for errors
	secret    string // the password, only to redact it from error messages
	opTimeout time.Duration

	sess  *smb2.Session
	share *smb2.Share

	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// Dial connects to cfg.Host, authenticates with NTLMv2 and mounts cfg.Share.
// It is bounded by cfg.DialTimeout. Errors never contain the password.
func Dial(ctx context.Context, cfg Config) (Client, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	target := cfg.target()

	ctx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	var domain *string // nil lets the server's challenge choose the domain
	if cfg.Domain != "" {
		d := cfg.Domain
		domain = &d
	}
	dialer := &smb2.Dialer{
		Credentials: auth.NTLMCredential{User: cfg.Username, Password: cfg.Password, Domain: domain},
		TransportDialer: smb2.TCPDialer{
			Port:   cfg.Port,
			Dialer: &net.Dialer{Timeout: cfg.DialTimeout, KeepAlive: 30 * time.Second},
		},
		// The Apple extension is only needed to manage security descriptors on
		// macOS servers; skipping it saves a CREATE on the share root.
		DisableAAPLExtension: true,
	}
	// Dial closes the transport when ctx ends, so it is bounded on its own.
	sess, err := dialer.Dial(ctx, cfg.Host)
	if err != nil {
		return nil, redactErr(fmt.Errorf("smb: connect to %s: %w", target, err), cfg.Password)
	}
	done := watchdog(ctx, abortGrace, func() { _ = sess.Abort() })
	share, err := sess.Mount(ctx, cfg.Share)
	done()
	if err != nil {
		_ = sess.Abort()
		return nil, redactErr(fmt.Errorf("smb: connect to %s: %w", target, err), cfg.Password)
	}
	return &smbClient{
		target:    target,
		secret:    cfg.Password,
		opTimeout: cfg.OpTimeout,
		sess:      sess,
		share:     share,
	}, nil
}

// ListDir implements Client.
func (c *smbClient) ListDir(ctx context.Context, dir string) ([]Entry, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	if c.closed.Load() {
		return nil, ErrClosed
	}
	ctx, done := c.begin(ctx)
	defer done()

	infos, err := c.share.ReadDir(ctx, dir)
	if err != nil {
		return nil, c.wrapErr(err)
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		e := Entry{
			Name:    info.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		}
		if st, ok := info.(*smb2.FileStat); ok {
			e.FileID = normalizeFileID(st.FileId)
			e.CreationTime = st.CreationTime
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ReadAt implements Client.
//
// Each chunk is one round trip: a compound CREATE + READ + CLOSE, so no handle
// outlives the request. Reads larger than readChunkSize reopen the file per
// chunk and stop if its FileId changed in between.
func (c *smbClient) ReadAt(ctx context.Context, p string, off int64, maxLen int) (ReadResult, error) {
	name, err := CleanPath(p)
	if err != nil {
		return ReadResult{}, err
	}
	if name == "" || off < 0 || maxLen < 0 {
		return ReadResult{}, &os.PathError{Op: "read", Path: p, Err: os.ErrInvalid}
	}
	if c.closed.Load() {
		return ReadResult{}, ErrClosed
	}
	ctx, done := c.begin(ctx)
	defer done()

	smbName := strings.ReplaceAll(name, "/", `\`)
	// One byte of each READ may be the overlap described in readRange.
	res, err := readLoop(off, maxLen, readChunkSize-1, func(off int64, n int) (chunk, error) {
		return c.readChunk(ctx, smbName, off, n)
	})
	if err != nil {
		return ReadResult{}, c.wrapErr(&os.PathError{Op: "read", Path: name, Err: err})
	}
	return res, nil
}

// readChunk opens name, reads up to n bytes at off and closes it, in one
// round trip unless the file is now shorter than off. n == 0 only opens and
// closes the file. n must be below readChunkSize, see readRange.
func (c *smbClient) readChunk(ctx context.Context, name string, off int64, n int) (chunk, error) {
	if n > 0 {
		start, length := readRange(off, n)
		res, err := c.share.Request().
			Create(name, readAccess, wire.FILE_OPEN, wire.FILE_NON_DIRECTORY_FILE, 0, wire.QueryOnDiskIDRequest{}).
			Read(uint32(length), uint64(start)).
			Close().
			Do(ctx)
		if err == nil {
			defer res.Close()
			ch, err := decodeCreate(res)
			if err != nil {
				return chunk{}, err
			}
			read, err := res.Read(1)
			if err != nil {
				return chunk{}, err
			}
			ch.data = readData(off, n, read.Data())
			return ch, nil
		}
		// The library closed the handle. A READ at or past the end of the file
		// fails the compound and takes the CREATE response with it, so open
		// the file again below to report its identity and size.
		if !readHitEndOfFile(err) {
			return chunk{}, err
		}
	}
	res, err := c.share.Request().
		Create(name, readAccess, wire.FILE_OPEN, wire.FILE_NON_DIRECTORY_FILE, 0, wire.QueryOnDiskIDRequest{}).
		Close().
		Do(ctx)
	if err != nil {
		return chunk{}, err
	}
	defer res.Close()
	return decodeCreate(res)
}

// readRange returns the READ that serves n bytes at off. A READ that starts
// at or past the end of the file fails ([MS-SMB2] 3.3.5.12), and learning the
// file's size then costs a second round trip. Starting one byte early, when
// there is a byte before off, lets the most frequent read succeed instead: a
// poll that finds nothing new at the end of the file gets that one byte back.
// readData drops it.
func readRange(off int64, n int) (start int64, length int) {
	if off > 0 {
		return off - 1, n + 1
	}
	return off, n
}

// readData returns the bytes at off, at most n, from the data returned by the
// READ that readRange chose.
func readData(off int64, n int, data []byte) []byte {
	if off > 0 && len(data) > 0 {
		data = data[1:]
	}
	return append([]byte(nil), data[:min(len(data), n)]...)
}

func decodeCreate(res *protocol.Response) (chunk, error) {
	created, err := res.Create(0)
	if err != nil {
		return chunk{}, err
	}
	ch := chunk{size: created.EndofFile()}
	if q := created.QueryOnDiskID(); q != nil {
		ch.fileID = normalizeFileID(q.DiskFileId())
	}
	return ch, nil
}

// readHitEndOfFile reports whether a CREATE + READ + CLOSE compound failed
// only because the READ started at or past the end of the file.
func readHitEndOfFile(err error) bool {
	var compound *protocol.CompoundResponseError
	if !errors.As(err, &compound) || compound.OpError(0) != nil {
		return false
	}
	readErr := protocol.ResponseErrorAt(err, 1)
	return readErr != nil && readErr.Code == statusEndOfFile
}

// Close logs off and closes the connection. LOGOFF makes the server close any
// tree and open still attached to the session ([MS-SMB2] 3.3.5.7); the library
// bounds it with its own timeout.
func (c *smbClient) Close() error {
	c.closed.Store(true)
	c.closeOnce.Do(func() { c.closeErr = c.wrapErr(c.sess.Close()) })
	return c.closeErr
}

// abort drops the connection without LOGOFF. The reconnecting client uses it
// for sessions it already knows are broken.
func (c *smbClient) abort() {
	c.closed.Store(true)
	c.closeOnce.Do(func() { c.closeErr = c.wrapErr(c.sess.Abort()) })
}

// String implements fmt.Stringer.
func (c *smbClient) String() string { return c.target }

// begin bounds one operation by the operation timeout, and aborts the session
// if the operation is still running abortGrace after its context ended. The
// returned function must be called when the operation returns.
func (c *smbClient) begin(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(ctx, c.opTimeout)
	done := watchdog(ctx, abortGrace, func() { _ = c.sess.Abort() })
	return ctx, func() {
		done()
		cancel()
	}
}

func (c *smbClient) wrapErr(err error) error {
	return redactErr(err, c.secret)
}

// chunk is one open-read-close of readChunk.
type chunk struct {
	fileID uint64
	size   int64
	data   []byte
}

// readLoop fills a ReadResult of up to maxLen bytes from off with reads of at
// most chunkSize bytes. It stops at the end of the file, on a short read, or
// when a later chunk sees a different file (other FileId, or a size below
// what was already read). An error on the first chunk is returned; a later
// one ends the read early so the bytes already read are not lost, and the
// next call reports the error again if it persists.
func readLoop(off int64, maxLen, chunkSize int, readChunk func(off int64, n int) (chunk, error)) (ReadResult, error) {
	var res ReadResult
	for first := true; ; first = false {
		pos := off + int64(len(res.Data))
		n := min(maxLen-len(res.Data), chunkSize)
		ch, err := readChunk(pos, n)
		if err != nil {
			if first {
				return ReadResult{}, err
			}
			return res, nil
		}
		if !first && (ch.fileID != res.FileID || ch.size < pos) {
			return res, nil
		}
		if len(ch.data) > n {
			ch.data = ch.data[:n]
		}
		res.FileID, res.Size = ch.fileID, ch.size
		res.Data = append(res.Data, ch.data...)
		if len(ch.data) < n || len(res.Data) >= maxLen || pos+int64(len(ch.data)) >= ch.size {
			return res, nil
		}
	}
}

// watchdog calls abort if ctx ends and done is not called within grace.
// done must be called once the guarded operation returns.
func watchdog(ctx context.Context, grace time.Duration, abort func()) (done func()) {
	var (
		mu       sync.Mutex
		finished bool
		timer    *time.Timer
	)
	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		defer mu.Unlock()
		if !finished {
			timer = time.AfterFunc(grace, abort)
		}
	})
	return func() {
		stop()
		mu.Lock()
		defer mu.Unlock()
		finished = true
		if timer != nil {
			timer.Stop()
		}
	}
}

// redactedError is an error whose message contained the password. It keeps
// only the redacted message and the classification of the original error, so
// unwrapping it cannot reveal the password either.
type redactedError struct {
	msg  string
	kind ErrorKind
}

func (e *redactedError) Error() string { return e.msg }

// redactErr returns err unchanged unless its message contains secret, in
// which case it returns a redactedError. The library never puts the password
// in an error; this is a backstop for the guarantee that it never reaches a
// log line or a status message.
func redactErr(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, secret) {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(msg, secret, redactedSecret), kind: Classify(err)}
}
