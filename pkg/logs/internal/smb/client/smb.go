// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
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

	// listBatch is the number of entries of one read of a directory, and
	// maxListEntries the most entries ListDir returns. A directory with more
	// is refused with ErrTooManyEntries: the scanner lists every directory of
	// a source's pattern on each poll, so an unbounded listing would be
	// rebuilt in memory every second, from a legitimate directory with
	// millions of files or from a server that sends entries without end. The
	// limit is far above what a log directory holds (the launcher tails at most
	// logs_config.open_files_limit files, 500 by default).
	//
	// maxListBytes bounds the memory of the entries as well, since a count
	// alone does not: a server can send entries whose names fill a whole
	// directory page, 64 KiB, each. It is refused with ErrListingTooLarge. An
	// entry costs its name plus entryOverhead, the size of the struct and of the
	// library's file information.
	//
	// A read of listBatch entries may collect one directory page of up to
	// 64 KiB per entry before the limits are checked (4 MiB), and the library
	// decodes the entries of those pages into file information, so the batch
	// bounds how far a server gets past them: about 6 MiB at most.
	//
	// maxNameUnits is the longest name an entry may have, in UTF-16 code units:
	// the longest a file name is on NTFS, ReFS, SMB and Samba. A longer one is
	// refused with ErrNameTooLong.
	listBatch      = 64
	maxListEntries = 100_000
	maxListBytes   = 64 << 20
	entryOverhead  = 128
	maxNameUnits   = 255

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

	// Dial closes the transport when ctx ends, so it is bounded on its own.
	sess, err := newDialer(cfg).Dial(ctx, cfg.Host)
	if err != nil {
		return nil, dialError(err, cfg, target)
	}
	done := watchdog(ctx, abortGrace, func() { _ = sess.Abort() })
	share, err := sess.Mount(ctx, cfg.Share)
	done()
	if err != nil {
		_ = sess.Abort()
		return nil, checked(fmt.Errorf("smb: connect to %s: %w", target, redactErr(err, cfg.Password)))
	}
	if err := checkEncryption(share, cfg.RequireEncryption); err != nil {
		_ = sess.Abort()
		return nil, checked(fmt.Errorf("smb: connect to %s: %w", target, err))
	}
	return &smbClient{
		target:    target,
		secret:    cfg.Password,
		opTimeout: cfg.OpTimeout,
		sess:      sess,
		share:     share,
	}, nil
}

// dialError returns the error of a failed session setup: the library's error
// without the password, with the Agent's own text around it. The Agent's text is
// added after the password is removed from the library's, so that a password
// that happens to be a word of it cannot take the Agent's text away.
func dialError(err error, cfg Config, target string) error {
	out := redactErr(err, cfg.Password)
	if isGuestSession(err) {
		out = fmt.Errorf("%w: %w", errGuestSession, out)
	} else {
		out = negotiateHint(out, cfg)
	}
	return checked(fmt.Errorf("smb: connect to %s: %w", target, out))
}

// checked marks err as built by Dial: the library's text in it was already
// searched for the password, and the Agent's own text must not be.
func checked(err error) error { return &checkedError{err} }

type checkedError struct{ error }

func (e *checkedError) Unwrap() error { return e.error }

// smb3Dialects are the dialects the Agent offers unless the source sets
// allow_smb2. A server that supports none of them answers the negotiation with
// STATUS_NOT_SUPPORTED. Restricting the list keeps an attacker who can alter
// the unauthenticated negotiation from forcing a downgrade to SMB 2, whose
// sessions cannot be encrypted. SMB 3.1.1 protects the negotiation itself
// (pre-authentication integrity); 3.0 and 3.0.2 are kept for Windows Server
// 2012 and 2012 R2, and Samba before 4.3.
var smb3Dialects = []smb2.Dialect{smb2.SMB311, smb2.SMB302, smb2.SMB300}

func dialects(cfg Config) []smb2.Dialect {
	if cfg.AllowSMB2 {
		return nil // the library's list: SMB 3.1.1 down to 2.0.2
	}
	return smb3Dialects
}

// negotiateHint adds to a dial error the hint that fits a server which supports
// none of the dialects offered, which answers the negotiation with
// STATUS_NOT_SUPPORTED.
func negotiateHint(err error, cfg Config) error {
	if code, ok := statusCode(err); ok && code == statusNotSupported && !cfg.AllowSMB2 {
		return fmt.Errorf("%w (the server may support only SMB 2, which the Agent does not offer by default: set allow_smb2: true in the source's smb block to allow it, preferably after upgrading the server)", err)
	}
	return err
}

// ErrNotEncrypted is the error of a dial to a server that does not encrypt the
// session or the share while the source requires it. It classifies as ErrAuth,
// so it is retried every 30 seconds: the server's configuration, or the
// source's, needs a change, and the credentials were accepted.
var ErrNotEncrypted = errors.New("the server does not encrypt the session or the share, which require_encryption demands: enable encryption on the server (for example Windows 'Encrypt data access' on the share, or Samba 'server smb encrypt = required'), or turn require_encryption off for a network you trust")

// checkEncryption fails closed when the source requires encryption and the
// mounted share is not encrypted.
func checkEncryption(share interface{ Encrypted() bool }, require bool) error {
	if require && !share.Encrypted() {
		return ErrNotEncrypted
	}
	return nil
}

// newDialer returns the dialer of Dial. cfg has its defaults applied.
func newDialer(cfg Config) *smb2.Dialer {
	var domain *string // nil lets the server's challenge choose the domain
	if cfg.Domain != "" {
		d := cfg.Domain
		domain = &d
	}
	return &smb2.Dialer{
		Credentials: auth.NTLMCredential{User: cfg.Username, Password: cfg.Password, Domain: domain},
		TransportDialer: smb2.TCPDialer{
			Port:   cfg.Port,
			Dialer: &net.Dialer{Timeout: cfg.DialTimeout, KeepAlive: 30 * time.Second},
		},
		// Signing (or encryption, which replaces it on SMB 3) authenticates
		// every response with a key that only a server that verified the
		// password can derive. Without it, a server that grants a guest or
		// anonymous session (Samba's "map to guest", for a mistyped username)
		// or a machine answering in the server's place could serve any content
		// as the share's files. Requiring it also makes the library reject
		// guest and anonymous sessions ([MS-SMB2] 3.2.5.3.1), see
		// isGuestSession.
		RequireMessageSigning: true,
		// Only SMB 3 unless the source opts into SMB 2 (see smb3Dialects).
		SpecifiedDialects: dialects(cfg),
		// The Apple extension is only needed to manage security descriptors on
		// macOS servers; skipping it saves a CREATE on the share root.
		DisableAAPLExtension: true,
	}
}

// errGuestSession is the error of a dial that the server would only grant as a
// guest or anonymous session. It classifies as ErrAuth.
var errGuestSession = errors.New("the server only granted a guest or anonymous session, which cannot sign messages and is refused: check the username and password")

// isGuestSession reports whether a dial failed because the server granted a
// guest or anonymous session while signing is required. The vendored library
// reports it with these messages (x/protocol/session.go validateSessionFlags).
func isGuestSession(err error) bool {
	var invalid *protocol.InvalidResponseError
	if !errors.As(err, &invalid) {
		return false
	}
	return strings.HasPrefix(invalid.Message, "guest account ") || strings.HasPrefix(invalid.Message, "anonymous account ")
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

	f, err := c.share.OpenDir(ctx, dir)
	if err != nil {
		return nil, c.wrapErr(err)
	}
	// Close the handle even when ctx ended, so the server does not keep it
	// open; the library bounds the call.
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), abortGrace)
		defer cancel()
		_ = f.Close(closeCtx)
	}()

	entries, err := readEntries(func(n int) ([]os.FileInfo, error) { return f.Readdir(ctx, n) }, listBatch, maxListEntries, maxListBytes)
	if err != nil {
		switch {
		case errors.Is(err, ErrTooManyEntries):
			return nil, TooManyEntries(dir)
		case errors.Is(err, ErrListingTooLarge):
			return nil, ListingTooLarge(dir)
		case errors.Is(err, ErrNameTooLong):
			return nil, NameTooLong(dir)
		}
		return nil, c.wrapErr(err)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

// entryCost is the memory an entry of a listing is counted for.
func entryCost(name string) int { return len(name) + entryOverhead }

// ListingCost returns what the entries of a listing count for against the limit
// on the memory of listings (see maxListBytes), the same way ListDir counts
// them: callers that list many directories bound them together with it.
func ListingCost(entries []Entry) int {
	cost := 0
	for _, e := range entries {
		cost += entryCost(e.Name)
	}
	return cost
}

// MaxListEntries and MaxListBytes are the most entries, and the most memory in
// the sense of ListingCost, that one listing may have.
const (
	MaxListEntries = maxListEntries
	MaxListBytes   = maxListBytes
)

// readEntries reads a directory with readBatch, which returns up to n entries
// (io.EOF, or fewer than n, at the end), batch entries at a time. It returns
// ErrTooManyEntries as soon as the directory has more than limit entries,
// ErrListingTooLarge as soon as they take more than maxBytes (see
// maxListBytes), and ErrNameTooLong at the first name of more than
// maxNameUnits UTF-16 code units, without reading the rest: it never returns
// part of a directory as if it were all of it.
func readEntries(readBatch func(n int) ([]os.FileInfo, error), batch, limit, maxBytes int) ([]Entry, error) {
	var (
		entries []Entry
		bytes   int
	)
	for {
		infos, err := readBatch(batch)
		for _, info := range infos {
			name := info.Name()
			if utf16Len(name) > maxNameUnits {
				return nil, ErrNameTooLong
			}
			bytes += entryCost(name)
			e := Entry{
				Name:    name,
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
		switch {
		case len(entries) > limit:
			return nil, ErrTooManyEntries
		case bytes > maxBytes:
			return nil, ErrListingTooLarge
		}
		switch {
		case errors.Is(err, io.EOF):
			return entries, nil
		case err != nil:
			return nil, err
		case len(infos) < batch:
			return entries, nil // a short batch is the last one
		}
	}
}

// utf16Len returns the length of s in UTF-16 code units, which is how SMB
// limits a name.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++ // a surrogate pair
		}
	}
	return n
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
	ch := chunk{size: created.EndofFile(), created: created.CreationTime().Time()}
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
// bounds it with its own timeout (5s), and Abort cuts it short.
func (c *smbClient) Close() error {
	c.closed.Store(true)
	c.closeOnce.Do(func() { c.closeErr = c.wrapErr(c.sess.Close()) })
	return c.closeErr
}

// Abort drops the connection without LOGOFF, see the Abort function. It cuts
// short a concurrent Close waiting for its LOGOFF, so it never waits for the
// server.
func (c *smbClient) Abort() error {
	c.closed.Store(true)
	// The library tears the connection down before it joins a Close in
	// progress, whose LOGOFF then fails at once.
	err := c.wrapErr(c.sess.Abort())
	c.closeOnce.Do(func() { c.closeErr = err })
	return err
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
	fileID  uint64
	created time.Time
	size    int64
	data    []byte
}

// readLoop fills a ReadResult of up to maxLen bytes from off with reads of at
// most chunkSize bytes. It stops at the end of the file, on a short read, or
// when a later chunk sees a different file (other FileId or creation time, or
// a size below what was already read). An error on the first chunk is
// returned; a later one ends the read early so the bytes already read are not
// lost, and the next call reports the error again if it persists.
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
		if !first && (ch.fileID != res.FileID || !ch.created.Equal(res.CreationTime) || ch.size < pos) {
			return res, nil
		}
		if len(ch.data) > n {
			ch.data = ch.data[:n]
		}
		res.FileID, res.CreationTime, res.Size = ch.fileID, ch.created, ch.size
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
	var built *checkedError
	if errors.As(err, &built) {
		return err // built by Dial, which searched the library's text
	}
	msg := err.Error()
	if !strings.Contains(msg, secret) {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(msg, secret, redactedSecret), kind: Classify(err)}
}
