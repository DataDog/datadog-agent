package protocol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

func newHashContext() (*wire.HashContext, error) {
	hc := &wire.HashContext{
		HashAlgorithms: clientHashAlgorithms,
		HashSalt:       make([]byte, 32),
	}
	if _, err := rand.Read(hc.HashSalt); err != nil {
		return nil, fmt.Errorf("protocol: generate preauthentication salt: %w", err)
	}
	return hc, nil
}

func newCipherContext(ciphers []Cipher) *wire.CipherContext {
	if len(ciphers) == 0 {
		ciphers = clientCiphers
	}
	return &wire.CipherContext{
		Ciphers: ciphers,
	}
}

func newCompressionContext() *wire.CompressionContext {
	return &wire.CompressionContext{
		CompressionAlgorithms: clientCompressionAlgorithms,
		Flags:                 wire.SMB2_COMPRESSION_CAPABILITIES_FLAG_NONE,
	}
}

const (
	directStateIdle     uint32 = 0
	directStateReading  uint32 = 1
	directStateDone     uint32 = 2
	directStateCanceled uint32 = 3
)

type outstandingRequest struct {
	payloadRequest payloadRequest
	msgId          uint64
	asyncId        atomic.Uint64
	cmd            wire.Command
	ctx            context.Context
	recv           chan *recvPacket
	err            error
	canceled       atomic.Bool
	cancelOnce     sync.Once
	// requireEncryption records Request.IsEncrypted. [MS-SMB2] 3.3.4.1.4
	// requires every Response to such a request to be encrypted. The send
	// paths derive this from session ([MS-SMB2] 2.2.6) and share policy,
	// preserving negotiation, SESSION_SETUP and share TREE_CONNECT exceptions.
	// Keep it per request so compound and async responses cannot lose it.
	requireEncryption bool
	creditCharge      uint16
	// expectedRead/Write are the lengths advertised by the corresponding
	// request. They are used to reject a successful response which claims to
	// transfer more data than was requested.
	expectedRead     uint32
	hasExpectedRead  bool
	expectedWrite    uint32
	hasExpectedWrite bool
	lockWait         bool
	// waitFinal preserves final responses after cancellation so the owning
	// layer can reclaim handles or trees that the server did not cancel.
	waitFinal bool

	// readBuf is the caller-provided buffer that the payload of a direct
	// I/O READ Response is received into. It is registered by
	// makeOutstandingRequest and consumed directly by the transport.
	readBuf []byte

	// directDone is closed when direct reception into readBuf has finished
	// (either successfully or aborted by error).
	directDone  chan struct{}
	directOnce  sync.Once
	directState atomic.Uint32
}

func (rr *outstandingRequest) finishDirect() {
	if rr.directDone != nil {
		rr.directOnce.Do(func() {
			close(rr.directDone)
		})
	}
}

func (rr *outstandingRequest) abort() {
	rr.canceled.Store(true)

	if rr.directState.Swap(directStateCanceled) == directStateReading {
		// A direct read is currently reading directly into the caller's
		// buffer. Wait for in-flight reception to complete so late bytes
		// never overwrite the returned buffer.
		<-rr.directDone
	} else {
		rr.finishDirect()
	}

	select {
	case rp := <-rr.recv:
		if rp != nil {
			rp.close()
		}
	default:
	}
}

type outstandingRequests struct {
	m        sync.Mutex
	requests map[uint64]*outstandingRequest
}

func newOutstandingRequests() *outstandingRequests {
	return &outstandingRequests{
		requests: make(map[uint64]*outstandingRequest),
	}
}

func (r *outstandingRequests) pop(msgId uint64) (*outstandingRequest, bool) {
	r.m.Lock()
	defer r.m.Unlock()

	rr, ok := r.requests[msgId]
	if !ok {
		return nil, false
	}

	delete(r.requests, msgId)

	return rr, true
}

func (r *outstandingRequests) peek(msgId uint64) (*outstandingRequest, bool) {
	r.m.Lock()
	defer r.m.Unlock()

	rr, ok := r.requests[msgId]
	return rr, ok
}

func (r *outstandingRequests) set(msgId uint64, rr *outstandingRequest) {
	r.m.Lock()
	defer r.m.Unlock()

	r.requests[msgId] = rr
}

func (r *outstandingRequests) shutdown(err error) {
	r.m.Lock()
	defer r.m.Unlock()

	for _, rr := range r.requests {
		rr.err = err
		rr.finishDirect()
		close(rr.recv)
	}
	clear(r.requests)
}

type conn struct {
	t Transport

	session                    *session
	outstandingRequests        *outstandingRequests
	dialect                    uint16
	maxTransactSize            uint32
	maxReadSize                uint32
	maxWriteSize               uint32
	compressionIds             []uint16
	supportsChainedCompression bool
	ioPipelineDepth            uint
	requireSigning             bool
	capabilities               uint32
	preauthIntegrityHashId     uint16
	preauthIntegrityHashValue  [64]byte
	cipherId                   uint16
	acceptTransportSecurity    bool

	account *account

	transportClosed atomic.Bool
	writeTimeout    time.Duration
	receiverDone    chan struct{}

	m              sync.Mutex
	transportClose sync.Once
	transportErr   error

	err error

	_useSession atomic.Int32 // receiver use session?

	// Reusable packet transformation buffers. Use them with conn.m held.

	encodeBuf      []byte
	compressionBuf []byte
	encryptBuf     []byte
}

func (conn *conn) allocEncodeBuf(size int) []byte {
	return conn.allocBuf(&conn.encodeBuf, size)
}

func (conn *conn) allocEncryptBuf(size int) []byte {
	return conn.allocBuf(&conn.encryptBuf, size)
}

func (conn *conn) allocCompressionBuf(size int) []byte {
	return conn.allocBuf(&conn.compressionBuf, size)
}

func (conn *conn) allocBuf(buf *[]byte, size int) []byte {
	if cap(*buf) < size {
		newCap := max(size, clientMinBufSize)
		*buf = make([]byte, newCap)
	} else {
		clear((*buf)[:size])
	}
	*buf = (*buf)[:size]
	return *buf
}

func updatePreauthHash(hashVal *[64]byte, pkt []byte) {
	h := sha512.New()
	h.Write(hashVal[:])
	h.Write(pkt)
	h.Sum(hashVal[:0])
}

func (conn *conn) useSession() bool {
	return conn._useSession.Load() != 0
}

func (conn *conn) enableSession() {
	conn._useSession.Store(1)
}

func (conn *conn) maxCreditSize(companions int) int {
	if conn.account == nil {
		return maxSingleCreditPayloadSize
	}
	// Budget whole credit-sized payloads within the 24-bit transport length
	// ([MS-SMB2] 2.1), leaving 65535 bytes for headers and transforms. Exact
	// compound and transformed packet sizes are still checked before sending.
	credits := min(int(conn.account.maxCreditCap()), maxDirectTCPSize/maxSingleCreditPayloadSize)
	credits = max(credits-max(companions, 0), 1)
	return credits * maxSingleCreditPayloadSize
}

func (conn *conn) effectivePayloadSize(limit uint32, companions int) int {
	if limit == 0 {
		limit = maxSingleCreditPayloadSize
	}
	creditSize := conn.maxCreditSize(companions)
	if conn.capabilities&wire.SMB2_GLOBAL_CAP_LARGE_MTU == 0 {
		creditSize = min(creditSize, maxSingleCreditPayloadSize)
	}
	// Clamp before converting to int, including on 32-bit platforms.
	return int(min(limit, uint32(creditSize)))
}

func (conn *conn) closeLocked(err error) {
	if conn.err != nil {
		return
	}
	if err == nil {
		err = &TransportError{Err: net.ErrClosed}
	}
	conn.err = err

	if conn.account != nil {
		conn.account.abort(err)
	}
}

func (conn *conn) close(err error) error {
	// Close the transport before acquiring conn.m. A synchronous sender may
	// hold conn.m while blocked in transport I/O, so taking the lock first can
	// deadlock. The transport belongs to conn, so closing it here is the
	// connection layer tearing down its own resource.
	errClose := conn.closeTransport()
	conn.m.Lock()
	conn.closeLocked(err)
	conn.m.Unlock()
	conn.waitReceiver()
	return errClose
}

func (conn *conn) closeTransport() error {
	if conn == nil || conn.t == nil {
		return nil
	}
	conn.transportClose.Do(func() {
		// Closing the transport is what unblocks the receiver, so mark the
		// close as intentional first and let the receiver treat its resulting
		// read error as expected rather than reporting it as a fault.
		conn.transportClosed.Store(true)
		conn.transportErr = conn.t.Close()
	})
	return conn.transportErr
}

func (conn *conn) sendRecv(ctx context.Context, reqs ...wire.Packet) (*Response, error) {
	rrs, err := conn.send(ctx, false, reqs...)
	if err != nil {
		return nil, err
	}
	return recvAll(rrs, conn)
}

/*
mustSign returns true if req needs to be signed.

MS-SMB2 3.2.4.1.1 describes when a message needs to be signed.
https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/973630a8-8aa1-4398-89a8-13cf830f194d
*/
func (conn *conn) mustSign(s *session, req wire.Packet) bool {
	if _, isSessionSetup := req.(*wire.SessionSetupRequest); isSessionSetup {
		return false
	}

	// a 'guest' or anonymous user or a session without a key can't sign requests
	if s.signingDisabled() {
		return false
	}

	// true if the library user requested it at initialization or if the server
	// requires it
	if conn.requireSigning {
		return true
	}

	// Only SMB 3.1.1 requires TREE_CONNECT to always be signed, but for
	// simplicity's sake, we'll sign it no matter the dialect version.
	_, isTreeConnect := req.(*wire.TreeConnectRequest)
	return isTreeConnect
}

func (conn *conn) send(ctx context.Context, encrypt bool, reqs ...wire.Packet) (rrs []*outstandingRequest, err error) {
	charges, totalCreditCharge, err := conn.account.reserve(ctx, reqs...)
	if err != nil {
		return nil, err
	}

	conn.m.Lock()

	if conn.err != nil {
		err := conn.err
		conn.account.unloan(totalCreditCharge)
		conn.m.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		conn.account.unloan(totalCreditCharge)
		conn.m.Unlock()
		return nil, ctx.Err()
	default:
		// do nothing
	}

	msgIds := conn.account.assignIDs(charges, reqs...)
	rrs, parts, err := conn.makeOutstandingRequest(ctx, encrypt, msgIds, reqs...)
	if err != nil {
		conn.account.rollbackIDs(totalCreditCharge)
		conn.account.unloan(totalCreditCharge)
		conn.m.Unlock()
		return nil, err
	}

	// Once frame transmission starts, wait for its completion so cancellation
	// can remain a separate SMB2 CANCEL request ([MS-SMB2] 3.2.4.24).
	// Only the transport timeout bounds this write; recv handles request
	// cancellation after a successful send.
	err = conn.sendRaw(parts...)
	if err != nil {
		conn.account.unloan(totalCreditCharge)
		terr := &TransportError{err}
		// the transport is broken, tear down the connection
		conn.closeLocked(terr)
		// The request stays registered so the receiver's normal completion
		// path (tryHandle or the runReceiver shutdown) can unregister it and
		// close directDone. Responses are matched to OutstandingRequests by
		// MessageId ([MS-SMB2] 3.2.5.1.2); popping it here would hide the
		// request and leave the direct reception wait below without a
		// completion. Release the connection lock first: the receiver needs
		// it to tear the connection down, and holding it here would deadlock
		// the wait below. A direct READ that already published its sink may
		// still be writing into the caller's buffer, so abort every request
		// before returning the buffer for reuse.
		conn.m.Unlock()
		_ = conn.closeTransport()
		for _, rr := range rrs {
			rr.abort()
		}
		return nil, terr
	}

	conn.m.Unlock()
	return rrs, nil
}

func (conn *conn) sendRaw(parts ...[]byte) error {
	timeout := conn.writeTimeout
	if timeout <= 0 {
		timeout = clientWriteTimeout
	}
	deadline := time.Now().Add(timeout)
	if err := conn.t.setWriteDeadline(deadline); err != nil {
		return err
	}
	defer conn.t.setWriteDeadline(time.Time{})

	_, err := conn.t.writev(parts...)
	return err
}

func (conn *conn) makeOutstandingRequest(ctx context.Context, encrypt bool, msgIds []uint64, reqs ...wire.Packet) (rrs []*outstandingRequest, parts [][]byte, err error) {
	encrypt = encrypt && !conn.acceptTransportSecurity
	s := conn.session
	rrs = make([]*outstandingRequest, len(reqs))
	compress := conn.useSession() && conn.compressionEnabled()
	for _, req := range reqs {
		if req.Command() == wire.SMB2_NEGOTIATE {
			compress = false
			break
		}
	}

	// Direct I/O write: a non-empty WRITE is encoded without first copying its
	// payload into the ordinary packet buffer. For an unencrypted message the
	// payload is sent as an extra transport segment. Encrypted messages still
	// need one contiguous plaintext input for AEAD, so the payload is copied
	// directly into the encryption buffer instead. An empty write is excluded:
	// its generic encoding carries an extra dangling byte (see
	// WriteRequest.Size), which the direct path would drop. At most one WRITE
	// is sent directly; the rest falls back to the generic path. Compression
	// transforms the complete SMB2 message ([MS-SMB2] 3.1.4.4), so compressed
	// writes also use the contiguous generic path.
	directIdx := -1
	if !compress {
		for i, req := range reqs {
			wr, ok := req.(*wire.WriteRequest)
			if !ok || wr.WriteChannelInfo != nil || len(wr.Data) == 0 {
				continue
			}
			if directIdx >= 0 {
				directIdx = -1
				break
			}
			directIdx = i
		}
	}

	var data []byte
	if directIdx >= 0 {
		data = reqs[directIdx].(*wire.WriteRequest).Data
	}

	// Compound request (len(reqs) > 1)
	var totalSize int
	fixedSpans := make([]int, len(reqs)) // bytes encoded into pkt (payload excluded for directIdx); NextCommand uses the full wire span
	for i, req := range reqs {
		span := req.Size()
		if span < 64 || span > maxDirectTCPSize {
			return nil, nil, fmt.Errorf("protocol: invalid packet size %d", span)
		}
		if i < len(reqs)-1 {
			span = wire.Roundup(span, 8)
		}
		if span > maxDirectTCPSize-totalSize {
			return nil, nil, fmt.Errorf("protocol: compound packet exceeds transport size")
		}
		if i < len(reqs)-1 {
			req.SetNextCommand(uint32(span))
		} else {
			req.SetNextCommand(0)
		}
		fixedSpans[i] = span
		totalSize += span
	}
	if directIdx >= 0 && !encrypt {
		fixedSpans[directIdx] -= len(data)
		totalSize -= len(data)
	}

	// wrHeaderLen is the length of the direct WRITE's fixed part encoded
	// into pkt: SMB2 header + fixed body, without payload (WriteChannelInfo
	// is required to be nil by the detection above).
	const wrHeaderLen = 64 + 48

	for i, req := range reqs {
		switch r := req.(type) {
		case *DirectReadRequest:
			if compress {
				r.Flags |= wire.SMB2_READFLAG_REQUEST_COMPRESSED
			}
		case *wire.ReadRequest:
			if compress {
				r.Flags |= wire.SMB2_READFLAG_REQUEST_COMPRESSED
			}
		}

		msgId := msgIds[i]

		if i == 0 {
			req.SetFlags(req.HeaderFlags() &^ wire.SMB2_FLAGS_RELATED_OPERATIONS)
		} else {
			req.SetFlags(req.HeaderFlags() | wire.SMB2_FLAGS_RELATED_OPERATIONS)
		}

		rr := &outstandingRequest{
			cmd:               req.Command(),
			payloadRequest:    describePayloadRequest(req),
			msgId:             msgId,
			ctx:               ctx,
			recv:              make(chan *recvPacket, 1),
			requireEncryption: s != nil && encrypt,
			creditCharge:      req.CreditCharge(),
			lockWait:          req.Command() == wire.SMB2_LOCK,
		}
		switch r := req.(type) {
		case *wire.ReadRequest:
			rr.expectedRead, rr.hasExpectedRead = r.Length, true
		case *DirectReadRequest:
			if r.ReadRequest != nil {
				rr.expectedRead, rr.hasExpectedRead = r.Length, true
			}
		case *wire.WriteRequest:
			rr.expectedWrite, rr.hasExpectedWrite = uint32(len(r.Data)), true
		}

		if drr, ok := req.(*DirectReadRequest); ok {
			rr.readBuf = drr.Buffer
			rr.directDone = make(chan struct{})
		}

		rrs[i] = rr
	}

	var pkt []byte
	var encryptBuf []byte
	if s != nil && encrypt && directIdx >= 0 {
		// Keep the plaintext immediately before encryption in the same buffer
		// that will hold the transformed packet. AEAD permits exact in-place
		// operation, avoiding an intermediate encoded packet for direct I/O.
		encryptBuf = conn.allocEncryptBuf(52 + totalSize + 16)
		pkt = encryptBuf[52 : 52+totalSize]
	} else {
		pkt = conn.allocEncodeBuf(totalSize)
	}

	off := 0
	for i, req := range reqs {
		if i == directIdx {
			// Encode only the fixed part of the request. Encode computes
			// Length and DataOffset from wr.Data even though the payload
			// does not fit in pkt, so the fixed bytes stay identical to
			// the contiguous encoding. The padding after the payload (if
			// any) is left zeroed by the buffer allocator.
			req.Encode(pkt[off : off+wrHeaderLen])
			if encrypt {
				copy(pkt[off+wrHeaderLen:off+wrHeaderLen+len(data)], data)
			}
		} else {
			req.Encode(pkt[off : off+fixedSpans[i]])
		}
		if req.Command() == wire.SMB2_QUERY_INFO || req.Command() == wire.SMB2_QUERY_DIRECTORY {
			description, err := describeQueryRequest(req.Command(), pkt[off+64:off+fixedSpans[i]])
			if err != nil {
				return nil, nil, err
			}
			rrs[i].payloadRequest = description
		}
		if req.Command() == wire.SMB2_IOCTL {
			r := wire.IoctlRequestDecoder(pkt[off+64 : off+fixedSpans[i]])
			if r.IsInvalid() {
				return nil, nil, errors.New("protocol: invalid encoded IOCTL request")
			}
			description := &rrs[i].payloadRequest
			description.command = wire.SMB2_IOCTL
			description.ctlCode = r.CtlCode()
			description.maxInput = r.MaxInputResponse()
			description.maxOutput = r.MaxOutputResponse()
		}
		// [MS-SMB2] 2.2.1.2 and 3.2.4.1.5 require a reserved zero wire
		// CreditCharge for SMB 2.0.2, without changing internal accounting.
		// Include SMB 2.0.2-only NEGOTIATE before the dialect is known, and
		// correct the header before signing, compression and encryption.
		zeroCreditCharge := conn.dialect == wire.SMB202
		if nr, ok := req.(*wire.NegotiateRequest); ok && len(nr.Dialects) == 1 && nr.Dialects[0] == wire.SMB202 {
			zeroCreditCharge = true
		}
		if zeroCreditCharge {
			wire.PacketCodec(pkt[off : off+64]).SetCreditCharge(0)
		}
		off += fixedSpans[i]
	}

	if s != nil && !encrypt {
		off = 0
		requireSigning := false
		for _, req := range reqs {
			if conn.mustSign(s, req) {
				requireSigning = true
				break
			}
		}
		for i := range reqs {
			subPkt := pkt[off : off+fixedSpans[i]]
			if requireSigning {
				if i == directIdx {
					// The signed region covers the payload in between,
					// matching the contiguous encoding.
					s.sign(subPkt[:wrHeaderLen], data, subPkt[wrHeaderLen:])
				} else {
					s.sign(subPkt)
				}
			}
			off += fixedSpans[i]
		}
	}

	// [MS-SMB2] 3.1.4.3 requires compression before encryption when both
	// transforms apply to the same message.
	if compress {
		compressionBuf := conn.allocCompressionBuf(maxCompressedPacketSize(len(pkt)))
		pkt, err = compressPacketInto(pkt, compressionBuf)
		if err != nil {
			return nil, nil, fmt.Errorf("protocol: compress packet: %w", err)
		}
	}

	if s != nil && encrypt {
		if encryptBuf == nil {
			encSize := 52 + len(pkt) + 16
			encryptBuf = conn.allocEncryptBuf(encSize)
		}
		pkt, err = s.encrypt(pkt, encryptBuf)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(pkt) > maxDirectTCPSize {
		return nil, nil, fmt.Errorf("protocol: packet exceeds transport size after transforms")
	}

	for _, rr := range rrs {
		conn.outstandingRequests.set(rr.msgId, rr)
	}

	if directIdx < 0 || encrypt {
		parts = [][]byte{pkt}
	} else {
		cut := 0
		for i := 0; i < directIdx; i++ {
			cut += fixedSpans[i]
		}
		cut += wrHeaderLen

		parts = make([][]byte, 0, 3)
		if cut > 0 {
			parts = append(parts, pkt[:cut])
		}
		parts = append(parts, data)
		if cut < len(pkt) {
			parts = append(parts, pkt[cut:])
		}
	}

	return rrs, parts, nil
}

func (conn *conn) recv(rr *outstandingRequest) (*recvPacket, error) {
	acceptResponse := func(rp *recvPacket) (*recvPacket, error) {
		if rp == nil {
			// the channel was closed by conn.close while rr was outstanding
			if rr.err != nil {
				return nil, rr.err
			}
			return nil, &TransportError{Err: net.ErrClosed}
		}
		if rr.err != nil {
			rp.close()
			return nil, rr.err
		}
		res, err := acceptRequest(rr, rp, conn.dialect)
		// Preserve local cancellation even when the server's cancellation
		// response arrives before the context's Done branch is selected.
		if rr.ctx.Err() != nil {
			if responseErr, ok := err.(*ResponseError); ok && responseErr.Code == uint32(erref.STATUS_CANCELLED) {
				return nil, rr.ctx.Err()
			}
		}
		return res, err
	}

	// A Response may have already arrived while the context was being
	// canceled: prefer the buffered Response over the cancellation.
	select {
	case rp := <-rr.recv:
		return acceptResponse(rp)
	default:
	}

	select {
	case rp := <-rr.recv:
		return acceptResponse(rp)
	case <-rr.ctx.Done():
		rr.cancelOnce.Do(func() { go conn.sendCancel(rr) })
		if !rr.lockWait && !rr.waitFinal {
			rr.abort()
			return nil, rr.ctx.Err()
		}

		// CREATE groups and TREE_CONNECT need final responses to reclaim resources
		// when the server cannot cancel ([MS-SMB2] 3.3.5.16).
		// [MS-SMB2] 3.2.5.13 returns the result of a LOCK even after
		// CANCEL. Keep the request registered so a final success is not
		// hidden as a context error and its credits are charged once.
		return acceptResponse(<-rr.recv)
	}
}

func (conn *conn) sendCancel(rr *outstandingRequest) {
	req := &wire.CancelRequest{}
	req.SetMessageId(rr.msgId)
	if asyncId := rr.asyncId.Load(); asyncId != 0 {
		req.SetFlags(wire.SMB2_FLAGS_ASYNC_COMMAND)
		req.AsyncId = asyncId
	}

	conn.m.Lock()
	if conn.err != nil {
		conn.m.Unlock()
		return
	}

	s := conn.session
	if rr.requireEncryption && s == nil {
		conn.m.Unlock()
		return
	}
	if s != nil {
		req.SetSessionId(s.sessionId)
	}

	pkt := conn.allocEncodeBuf(req.Size())
	req.Encode(pkt)

	if rr.requireEncryption {
		// [MS-SMB2] 3.2.4.1.8 does not exempt CANCEL from required
		// encryption, so do not send a plaintext fallback on failure.
		encryptBuf := conn.allocEncryptBuf(52 + len(pkt) + 16)
		var err error
		pkt, err = s.encrypt(pkt, encryptBuf)
		if err != nil {
			conn.m.Unlock()
			return
		}
	} else if s != nil {
		if !s.signingDisabled() {
			s.sign(pkt)
		}
	}

	if err := conn.sendRaw(pkt); err != nil {
		conn.closeLocked(&TransportError{err})
		conn.m.Unlock()
		_ = conn.closeTransport()
		return
	}
	conn.m.Unlock()
}

func (conn *conn) runReceiver() {
	var err error
	if conn.receiverDone != nil {
		defer close(conn.receiverDone)
	}

	// A panic should shutdown the connection
	defer func() {
		if r := recover(); r != nil {
			err = &InvalidResponseError{Message: fmt.Sprintf("receiver panic: %v", r)}
			conn.finishReceiver(err)
		}
	}()

	for {
		rp, e := conn.t.readPacket(conn.responseReadSink)
		if e != nil {
			err = &TransportError{e}

			goto exit
		}

		hasSession := conn.useSession()

		var isEncrypted bool

		if hasSession {
			var errDecrypt error
			rp, isEncrypted, errDecrypt = conn.tryDecrypt(rp)
			if errDecrypt != nil {
				rp.close()
				err = errDecrypt
				goto exit
			}
		}

		p := rp.codec()

		if hasSession {
			if s := conn.session; s != nil && s.sessionId != p.SessionId() {
				rp.close()
				err = &InvalidResponseError{Message: "unknown session id"}
				goto exit
			}
		} else if p.IsInvalidResponse() {
			rp.close()
			err = &InvalidResponseError{Message: "invalid packet header"}
			goto exit
		}

		for {
			// split must be called before tryHandle because tryHandle may
			// close rp.
			next := p.NextCommand()

			var sub *recvPacket
			if next != 0 {
				sub = rp.split(next)
				if sub == nil || sub.codec().IsInvalid() {
					rp.close()
					if sub != nil {
						sub.close()
					}
					err = &InvalidResponseError{Message: "invalid chained packet header"}
					goto exit
				}
			}

			var responseErr error
			if p.IsInvalidResponse() {
				responseErr = &InvalidResponseError{Message: "broken response packet format"}
			} else if hasSession {
				responseErr = conn.tryVerify(rp, isEncrypted)
			}

			if e := conn.tryHandle(rp, responseErr); e != nil {
				logger.Println("skip:", e)
			}

			if sub == nil {
				break
			}

			rp = sub
			p = rp.codec()
		}
	}

exit:
	if conn.transportClosed.Load() {
		err = nil
	} else {
		logger.Println("error:", err)
	}

	conn.finishReceiver(err)
}

// finishReceiver records the terminal connection error and wakes all request
// waiters before closing the transport. It never closes the transport while
// holding conn.m, because synchronous senders may hold that mutex while they
// are waiting for transport I/O to return.
func (conn *conn) finishReceiver(err error) {
	conn.m.Lock()
	conn.closeLocked(err)
	conn.outstandingRequests.shutdown(conn.err)
	conn.m.Unlock()
	_ = conn.closeTransport()
}

func (conn *conn) waitReceiver() {
	if conn != nil && conn.receiverDone != nil {
		<-conn.receiverDone
	}
}

// directReadSink inspects the packet head and returns the caller-owned
// buffer (and front-end header size) for a direct I/O READ Response.
func (conn *conn) directReadSink(head []byte, restSize int) ([]byte, int) {
	p := wire.PacketCodec(head)
	if p.IsInvalid() ||
		p.Command() != wire.SMB2_READ ||
		p.NextCommand() != 0 ||
		erref.NtStatus(p.Status()) != erref.STATUS_SUCCESS {
		return nil, 0
	}

	r := wire.ReadResponseDecoder(p.Body())
	if r.IsInvalidHeader() || hasInvalidReadFlags(r, conn.dialect) {
		return nil, 0
	}

	rr, ok := conn.outstandingRequests.peek(p.MessageId())
	if !ok || len(rr.readBuf) == 0 {
		return nil, 0
	}

	// [MS-SMB2] 2.2.20 defines DataOffset as one byte and DataLength as
	// four bytes; validate their relationship before converting DataLength
	// to int. The data must exactly fill the rest of the packet, be at least
	// one byte long, and fit in the caller's buffer.
	frontSize := int(r.DataOffset())
	dataLength := uint64(r.DataLength())
	pad := frontSize - 80
	if restSize < 0 || pad < 0 || uint64(pad)+dataLength != uint64(restSize) || !rr.acceptsDirectReadLength(dataLength) {
		return nil, 0
	}

	if rr.canceled.Load() || !rr.directState.CompareAndSwap(directStateIdle, directStateReading) {
		return nil, 0
	}
	return rr.readBuf[:int(dataLength)], frontSize
}

func (conn *conn) responseReadSink(head []byte, restSize int) ([]byte, int) {
	p := wire.PacketCodec(head)
	if p.IsInvalidResponse() {
		return nil, 0
	}

	// SessionSetup publishes authentication state with enableSession after
	// final verification. Until then, avoid session state and direct sinks;
	// [MS-SMB2] 3.2.5.1.3 requires a session lookup before accepting a response.
	if !conn.useSession() {
		return nil, 0
	}
	s := conn.session
	if s == nil || s.sessionId != p.SessionId() {
		return nil, 0
	}

	// [MS-SMB2] 3.3.4.1.4 requires encrypted responses to encrypted requests.
	if rr, ok := conn.outstandingRequests.peek(p.MessageId()); ok && rr.requireEncryption {
		return nil, 0
	}

	if !s.signingDisabled() &&
		(conn.requireSigning || p.Flags()&wire.SMB2_FLAGS_SIGNED != 0) {
		// [MS-SMB2] 3.2.5.1.3 requires failed signatures to be discarded.
		return nil, 0
	}

	return conn.directReadSink(head, restSize)
}

func accept(cmd wire.Command, rp *recvPacket, dialect uint16) (res *recvPacket, err error) {
	return acceptWithLimits(cmd, rp, dialect, 0, false, 0, false)
}

func acceptWithLimits(cmd wire.Command, rp *recvPacket, dialect uint16, expectedRead uint32, hasRead bool, expectedWrite uint32, hasWrite bool) (res *recvPacket, err error) {
	defer func() {
		err = withResponseCommand(err, cmd)
		if res == nil {
			rp.close()
		}
	}()

	p := rp.codec()

	if command := p.Command(); cmd != command {
		return nil, invalidResponse(cmd, fmt.Sprintf("expected command: %s, got %s", cmd.String(), command.String()))
	}

	status := erref.NtStatus(p.Status())

	switch status {
	case erref.STATUS_SUCCESS:
		if err := validateResponsePacket(cmd, rp, dialect, expectedRead, hasRead, expectedWrite, hasWrite); err != nil {
			return nil, err
		}
		return rp, nil

	case erref.STATUS_MORE_PROCESSING_REQUIRED:
		if cmd == wire.SMB2_SESSION_SETUP {
			if err := validateResponsePacket(cmd, rp, dialect, expectedRead, hasRead, expectedWrite, hasWrite); err != nil {
				return nil, err
			}
			return rp, nil
		}

	case erref.STATUS_BUFFER_OVERFLOW:
		switch cmd {
		case wire.SMB2_QUERY_INFO:
			r := wire.QueryInfoResponseDecoder(p.Body())
			if !r.IsInvalid() {
				return nil, &ResponseError{Code: uint32(status), data: [][]byte{append([]byte(nil), r.Output()...)}}
			}
		case wire.SMB2_IOCTL:
			r := wire.IoctlResponseDecoder(p.Body())
			if !r.IsInvalid() {
				return nil, &ResponseError{Code: uint32(status), data: [][]byte{append([]byte(nil), r.Output()...)}}
			}
		case wire.SMB2_READ:
			if err := validateResponseBody(cmd, p.Body(), dialect, expectedRead, hasRead, expectedWrite, hasWrite); err != nil {
				return nil, err
			}
			r := wire.ReadResponseDecoder(p.Body())
			return nil, &ResponseError{Code: uint32(status), data: [][]byte{append([]byte(nil), r.Data()...)}}
		}

	case erref.STATUS_NOTIFY_ENUM_DIR:
		if cmd == wire.SMB2_CHANGE_NOTIFY {
			if err := validateResponsePacket(cmd, rp, dialect, expectedRead, hasRead, expectedWrite, hasWrite); err != nil {
				return nil, err
			}
			return rp, nil
		}
	}

	if cmd == wire.SMB2_IOCTL {
		r := wire.IoctlResponseDecoder(p.Body())
		if !r.IsInvalid() {
			switch r.CtlCode() {
			case wire.FSCTL_SRV_COPYCHUNK, wire.FSCTL_SRV_COPYCHUNK_WRITE:
				// [MS-SMB2] 3.3.5.15.6.1 returns copy failures as IOCTL
				// responses. Section 3.2.5.14.3 preserves their status;
				// accompanying results (or INVALID_PARAMETER limits) are
				// not transferred bytes. Section 2.2.32 defines boundaries.
				return nil, &ResponseError{Code: uint32(status)}
			}
		}
	}

	return nil, acceptError(uint32(status), p.Body(), dialect)
}

// hasInvalidReadFlags reports whether a READ response carries a Flags value
// this non-RDMA client must reject. Callers must have validated the response
// header first. [MS-SMB2] 2.2.20 defines Reserved2/Flags as dialect-specific;
// for SMB 3.1.1 [MS-SMB2] 3.2.5.11 rejects RDMA_TRANSFORM on a non-RDMA
// transport, leaving only 0 valid here.
func hasInvalidReadFlags(r wire.ReadResponseDecoder, dialect uint16) bool {
	return dialect == wire.SMB311 && r.Flags() != 0
}

func acceptError(status uint32, res []byte, dialect uint16) error {
	r := wire.ErrorResponseDecoder(res)
	if r.IsInvalid() {
		return &InvalidResponseError{Message: "broken error response format"}
	}

	eData := r.ErrorData()
	isSizeError := erref.NtStatus(status) == erref.STATUS_BUFFER_TOO_SMALL || erref.NtStatus(status) == erref.STATUS_INFO_LENGTH_MISMATCH

	if count := r.ErrorContextCount(); count != 0 {
		data := make([][]byte, count)
		var requiredBufferLength uint32

		for i := range data {
			ctx := wire.ErrorContextResponseDecoder(eData)
			if ctx.IsInvalid() {
				return &InvalidResponseError{Message: "broken error context response format"}
			}

			contextData := ctx.ErrorData()
			data[i] = append([]byte(nil), contextData...)
			// [MS-SMB2] 2.2.2.2 / 3.2.5.17 carry the four-byte required length
			// in the SMB 3.1.1 Error Context (ErrorId 0).
			if isSizeError && r.ByteCount() == 12 && len(data) == 1 && i == 0 && ctx.ErrorId() == wire.SMB2_ERROR_ID_DEFAULT && len(contextData) == 4 && dialect == wire.SMB311 {
				requiredBufferLength = binary.LittleEndian.Uint32(contextData)
			}

			// the last error context need not be padded to the 8-byte boundary (MS-SMB2 2.2.2)
			if i == len(data)-1 {
				break
			}

			next64 := uint64(8) + (uint64(ctx.ErrorDataLength())+7)&^uint64(7)
			if next64 > uint64(len(eData)) {
				return &InvalidResponseError{Message: "broken error context response format"}
			}
			next := int(next64)
			eData = eData[next:]
		}
		return &ResponseError{
			Code:                 status,
			data:                 data,
			requiredBufferLength: requiredBufferLength,
		}
	}
	data := append([]byte(nil), eData...)
	err := &ResponseError{Code: status, data: [][]byte{data}}
	// Before SMB 3.1.1, [MS-SMB2] 2.2.2.2 / 3.2.5.17 carry the required length
	// as four bytes of the SMB2 ERROR Response data.
	if isSizeError && len(data) == 4 && dialect != wire.SMB311 {
		err.requiredBufferLength = binary.LittleEndian.Uint32(data)
	}
	return err
}

func (conn *conn) tryDecrypt(rp *recvPacket) (*recvPacket, bool, error) {
	p := rp.codec()
	if p.IsInvalid() {
		if len(rp.pkt) >= 4 && bytes.Equal(rp.pkt[:4], []byte(wire.MAGIC3)) {
			pkt, ext, err := decompressPacketForReceive(conn, rp.bytes(), conn.responseReadSink)
			if err != nil {
				return rp, false, err
			}
			rp.pkt = pkt
			rp.ext = ext
			return rp, false, nil
		}

		t := rp.transformCodec()
		if t.IsInvalid() {
			return rp, false, &InvalidResponseError{Message: "broken packet header format"}
		}

		if t.Flags() != wire.Encrypted {
			return rp, false, &InvalidResponseError{Message: "encrypted flag is not on"}
		}

		if conn.session == nil || conn.session.sessionId != t.SessionId() {
			return rp, false, &InvalidResponseError{Message: "unknown session id returned"}
		}

		pkt, err := conn.session.decrypt(rp.bytes())
		if err != nil {
			return rp, false, &InvalidResponseError{Message: err.Error()}
		}

		if len(pkt) >= 4 && bytes.Equal(pkt[:4], []byte(wire.MAGIC3)) {
			var ext []byte
			// Keep encrypted compressed data in receive-owned storage until the
			// inner session and direction checks succeed ([MS-SMB2] 3.2.5.1.1.1).
			pkt, ext, err = decompressPacketForReceive(conn, pkt, nil)
			if err != nil {
				return rp, true, err
			}
			rp.ext = ext
		}

		// [MS-SMB2] 3.2.5.1.1.1 requires disconnecting on a SessionId
		// mismatch after decompression and recommends it for uncompressed
		// compounds. Validate every element before delivering any Response.
		if err := validateEncryptedResponse(pkt, t.SessionId()); err != nil {
			return rp, true, err
		}

		rp.pkt = pkt
		conn.copyDecryptedReadPayload(rp)
		return rp, true, nil
	}

	return rp, false, nil
}

// validateEncryptedResponse checks the complete compound before any response
// is delivered or decrypted payload is copied into a caller's buffer.
func validateEncryptedResponse(pkt []byte, sessionID uint64) error {
	for cur := pkt; ; {
		codec := wire.PacketCodec(cur)
		if codec.IsInvalidResponse() {
			return &InvalidResponseError{Message: "broken decrypted packet format"}
		}
		if codec.SessionId() != sessionID {
			return &InvalidResponseError{Message: "unknown session id in encrypted Response"}
		}
		if codec.NextCommand() == 0 {
			return nil
		}
		cur = cur[codec.NextCommand():]
	}
}

// copyDecryptedReadPayload completes the direct I/O path for encrypted READ
// responses. The transform is authenticated as one contiguous message, so
// the AEAD must first produce the decrypted SMB2 packet. Once it does, copy
// only the READ payload into the caller's registered buffer and expose it as
// the response's direct segment.
func (conn *conn) copyDecryptedReadPayload(rp *recvPacket) {
	if rp.ext != nil {
		return
	}
	p := rp.codec()
	if p.SessionId() != conn.session.sessionId ||
		p.Command() != wire.SMB2_READ || p.NextCommand() != 0 ||
		erref.NtStatus(p.Status()) != erref.STATUS_SUCCESS {
		return
	}

	rr, ok := conn.outstandingRequests.peek(p.MessageId())
	if !ok || len(rr.readBuf) == 0 {
		return
	}

	r := wire.ReadResponseDecoder(p.Body())
	if r.IsInvalid() || hasInvalidReadFlags(r, conn.dialect) || !rr.acceptsDirectReadLength(uint64(r.DataLength())) {
		return
	}

	if rr.canceled.Load() || !rr.directState.CompareAndSwap(directStateIdle, directStateReading) {
		return
	}
	copy(rr.readBuf, r.Data())
	rp.ext = rr.readBuf[:r.DataLength()]
	// Publish completion only after the caller's buffer is safe to reuse,
	// preserving cancellation if it arrived during the copy.
	rr.directState.CompareAndSwap(directStateReading, directStateDone)
}

func (conn *conn) tryVerify(rp *recvPacket, isEncrypted bool) error {
	p := rp.codec()

	msgID := p.MessageId()

	if rr, ok := conn.outstandingRequests.peek(msgID); ok && rr.requireEncryption && !isEncrypted {
		// [MS-SMB2] 3.3.4.1.4 requires encryption for every Response to an
		// encrypted request, including interim asynchronous responses.
		return invalidResponse(rr.cmd, "encrypted response required")
	}

	// MS-SMB2 3.2.5.1.3 states that the client MUST skip signature processing if:
	// - MessageId is 0xFFFFFFFFFFFFFFFF
	// - Status in the SMB2 header is STATUS_PENDING
	// 		- 3.3.4.1.1 says servers should skip signing interim responses to async requests - STATUS_PENDING is an interim Response
	// - Client is using the SMB 3.x dialect and the message was successfully decrypted+authenticated (isEncrypted=true)
	if msgID == 0xFFFFFFFFFFFFFFFF {
		return nil
	}
	if erref.NtStatus(p.Status()) == erref.STATUS_PENDING {
		return nil
	}
	if isEncrypted {
		return nil
	}

	s := conn.session
	if s == nil {
		return &InvalidResponseError{Message: "packet received before session established"}
	}
	if s.sessionId != p.SessionId() {
		return &InvalidResponseError{Message: "packet for unknown session"}
	}

	// guest and anonymous sessions can't produce signatures, so they don't need to be verified
	if s.signingDisabled() {
		return nil
	}

	// verify if 1) the connection requires signing or 2) if the message itself is signed
	if conn.requireSigning || p.Flags()&wire.SMB2_FLAGS_SIGNED != 0 {
		if !s.verify(rp.pkt, rp.ext) {
			return &InvalidResponseError{Message: "packet failed signature verification"}
		}
		return nil
	}

	// the message was not signed AND signing is not required
	return nil
}

func (conn *conn) tryHandle(rp *recvPacket, e error) error {
	p := rp.codec()

	msgId := p.MessageId()

	rr, ok := conn.outstandingRequests.pop(msgId)
	if ok && e == nil && erref.NtStatus(p.Status()) == erref.STATUS_PENDING {
		e = conn.validateInterimResponse(rr, p)
	}
	switch {
	case !ok:
		// [MS-SMB2] 3.2.5.1.2 requires responses without a matching
		// OutstandingRequests entry to be discarded as invalid.
		rp.close()
		if e != nil {
			return e
		}
		return &InvalidResponseError{Message: "unknown message id returned"}
	case e != nil:
		// [MS-SMB2] 3.2.5.1.3 requires a response with a failed signature
		// verification to be discarded. Unloan the request's credit charge
		// without granting the unauthenticated CreditResponse.
		conn.account.unloan(rr.creditCharge)
		rr.finishDirect()
		rp.close()
		e = withResponseCommand(e, rr.cmd)
		rr.err = e

		if !rr.canceled.Load() {
			close(rr.recv)
		}
		return e
	case erref.NtStatus(p.Status()) == erref.STATUS_PENDING:
		// MS-SMB2 3.3.4.1.2 grants asynchronous credits in the interim
		// response. This request no longer promises future credits, even
		// though it stays outstanding until its final response arrives.
		conn.account.charge(p.CreditResponse(), rr.creditCharge)
		rr.creditCharge = 0
		// Read the validated identifier before releasing the receive buffer.
		rr.asyncId.Store(p.AsyncId())
		rp.close()
		conn.outstandingRequests.set(msgId, rr)
	default:
		conn.account.charge(p.CreditResponse(), rr.creditCharge)

		rr.finishDirect()

		if rr.canceled.Load() {
			rp.close()
			return nil
		}

		rr.recv <- rp

		// rr.ctx may have been canceled between the canceled check and the
		// send above. Drain the response back, otherwise nobody will close
		// it and its buffer leaks.
		if rr.canceled.Load() {
			select {
			case rp := <-rr.recv:
				if rp != nil {
					rp.close()
				}
			default:
			}
		}
	}

	return nil
}

// validateInterimResponse enforces [MS-SMB2] 3.3.4.2 before an interim
// response can change credits or asynchronous request state.
func (conn *conn) validateInterimResponse(rr *outstandingRequest, p wire.PacketCodec) error {
	if p.Command() != rr.cmd || p.Flags()&wire.SMB2_FLAGS_ASYNC_COMMAND == 0 || p.AsyncId() == 0 {
		return invalidResponse(rr.cmd, "invalid asynchronous interim header")
	}
	if previous := rr.asyncId.Load(); previous != 0 && previous != p.AsyncId() {
		return invalidResponse(rr.cmd, "interim response changed async id")
	}
	r := wire.ErrorResponseDecoder(p.Body())
	if r.IsInvalid() || r.ByteCount() != 0 || r.ErrorContextCount() != 0 {
		return invalidResponse(rr.cmd, "invalid asynchronous interim error body")
	}
	// Only the receiver assigns AsyncIds. Match them under the request-map
	// lock because sending and teardown may concurrently change its entries.
	conn.outstandingRequests.m.Lock()
	defer conn.outstandingRequests.m.Unlock()
	for _, other := range conn.outstandingRequests.requests {
		if other.asyncId.Load() == p.AsyncId() {
			return invalidResponse(rr.cmd, "interim response reused an outstanding async id")
		}
	}

	return nil
}

func (rr *outstandingRequest) acceptsDirectReadLength(length uint64) bool {
	return length > 0 && length <= uint64(len(rr.readBuf)) &&
		(!rr.hasExpectedRead || length <= uint64(rr.expectedRead))
}
