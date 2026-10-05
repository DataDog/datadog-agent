package protocol

import (
	"context"
	"math"
	"slices"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// Dialect represents an SMB dialect revision.
type Dialect = wire.Dialect

const (
	SMB202 = wire.SMB202
	SMB210 = wire.SMB210
	SMB300 = wire.SMB300
	SMB302 = wire.SMB302
	SMB311 = wire.SMB311
)

// Cipher represents an SMB 3.x encryption cipher algorithm ID.
type Cipher = wire.Cipher

const (
	AES128CCM = wire.AES128CCM
	AES128GCM = wire.AES128GCM
	AES256CCM = wire.AES256CCM
	AES256GCM = wire.AES256GCM
)

// Dialer negotiates and authenticates independent SMB sessions. It may be used
// concurrently; do not modify it or referenced configuration while in use.
type Dialer struct {
	MaxCreditBalance uint16
	// IOPipelineDepth limits outstanding requests per Read/Write operation,
	// not per connection. Zero uses 4; 1 processes chunks sequentially.
	// Values above 65535 are invalid.
	IOPipelineDepth uint
	// RequireMessageSigning requires SMB message signing.
	RequireMessageSigning bool
	// ClientGuid identifies this client. If zero (uuid.Nil), a version 4
	// UUID is generated for each connection.
	ClientGuid uuid.UUID
	// SpecifiedDialects restricts negotiation to these SMB dialects. Empty
	// offers all supported client dialects ([MS-SMB2] 3.2.4.2). QUIC requires
	// SMB 3.1.1.
	SpecifiedDialects []Dialect
	// Ciphers restricts encryption to these cipher IDs in order of preference.
	// Empty offers client defaults ([MS-SMB2] 3.2.4.2.2).
	// A nonempty list must include AES128CCM to enable SMB 3.0/3.0.2 encryption.
	Ciphers []Cipher
	// DisableEncryptionOverSecureTransport offers QUIC transport security in
	// place of SMB encryption. SMB encryption is skipped only if the server
	// accepts the offer; this option has no effect on other transports.
	DisableEncryptionOverSecureTransport bool
}

// Dial negotiates and authenticates on t, transferring ownership of t to
// the returned Session. Supply a fresh initiator for each session.
func (d *Dialer) Dial(ctx context.Context, initiator Initiator, t Transport) (*Session, error) {
	if ctx == nil {
		panic("nil context")
	}
	if t == nil {
		panic("protocol: nil Transport")
	}
	if d == nil {
		_ = t.Close()
		panic("protocol: nil Dialer")
	}
	if initiator == nil {
		_ = t.Close()
		panic("protocol: nil Initiator")
	}
	if t.transportType() == "quic" {
		if len(d.SpecifiedDialects) > 0 && !slices.Contains(d.SpecifiedDialects, SMB311) {
			// [MS-SMB2] 2.1: SMB over QUIC requires the SMB 3.1.1 dialect. The
			// transport has not been published to the caller, so Dial still owns
			// it and must close it before panicking.
			_ = t.Close()
			panic("protocol: QUIC transport requires SMB 3.1.1")
		}
	}
	// [MS-SMB2] 3.2.4.2 requires valid SpecifiedDialects. Dial still owns
	// the transport during configuration validation, before connect takes over.
	for _, dialect := range d.SpecifiedDialects {
		if !slices.Contains(clientDialects, dialect) {
			_ = t.Close()
			panic("protocol: unsupported dialect specified")
		}
	}
	for _, cipher := range d.Ciphers {
		if !slices.Contains(clientCiphers, cipher) {
			_ = t.Close()
			panic("protocol: unsupported cipher specified")
		}
	}
	// At least one credit is needed per outstanding request. The account
	// uses uint16 balances, so a deeper pipeline cannot increase concurrency.
	if d.IOPipelineDepth > math.MaxUint16 {
		_ = t.Close()
		panic("protocol: I/O pipeline depth exceeds 65535")
	}
	// A caller's context must be able to terminate synchronous negotiation or
	// authentication I/O. The unpublished transport belongs to this Dial until
	// the session is returned.
	//
	// context.AfterFunc's stop does not wait for a callback that has already
	// started, so the watcher must be joined before ownership can be returned.
	// Otherwise the callback could still close the transport after Dial returns
	// and hand the caller a dead Session.
	watchDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(watchDone)
		_ = t.Close()
	})
	ws, err := d.connect(ctx, initiator, t)
	// Stop the watcher and wait for any in-flight Close to complete before
	// deciding whether the transport may be published. stop reports false when
	// the callback already started, in which case watchDone signals its end.
	if !stop() {
		<-watchDone
	}
	if err != nil {
		return nil, err
	}
	// Treat cancellation observed before ownership is handed to the caller as
	// a failed Dial. Once this function returns, no watcher remains that could
	// close the caller-owned Session.
	if err := ctx.Err(); err != nil {
		_ = ws.Close()
		return nil, err
	}
	return ws, nil
}

// connect negotiates and authenticates on t, which is owned by this Dial
// until it returns.
func (d *Dialer) connect(ctx context.Context, initiator Initiator, t Transport) (*Session, error) {
	maxCreditBalance := d.MaxCreditBalance
	if maxCreditBalance == 0 {
		maxCreditBalance = clientMaxCreditBalance
	}

	conn, err := d.negotiate(ctx, t, openAccount(maxCreditBalance))
	if err != nil {
		return nil, err
	}

	s, err := conn.sessionSetup(ctx, initiator)
	if err != nil {
		conn.close(err)
		return nil, err
	}

	return &Session{s: s}, nil
}

func (d *Dialer) negotiate(ctx context.Context, t Transport, a *account) (c *conn, err error) {
	t.setPacketReadTimeout(clientPacketReadTimeout)
	conn := &conn{
		t:                   t,
		outstandingRequests: newOutstandingRequests(),
		account:             a,
		receiverDone:        make(chan struct{}),
		writeTimeout:        clientWriteTimeout,
		ioPipelineDepth:     d.IOPipelineDepth,
	}

	defer func() {
		if err != nil {
			conn.close(err)
		}
	}()

	go conn.runReceiver()

	isQUIC := t.transportType() == "quic"

	dialects := d.SpecifiedDialects
	if len(dialects) == 0 {
		dialects = clientDialects
	}
	if isQUIC {
		dialects = []Dialect{SMB311}
	}

	req, err := d.makeNegotiateRequest(dialects, isQUIC && d.DisableEncryptionOverSecureTransport)
	if err != nil {
		return nil, err
	}

	res, err := conn.sendRecv(ctx, req)
	if err != nil {
		return nil, err
	}
	defer res.close()

	r := wire.NegotiateResponseDecoder(res.data(0))
	if r.IsInvalid() {
		return nil, invalidResponse(wire.SMB2_NEGOTIATE, "broken negotiate response format")
	}

	// Don't accept wildcard nor UnknownSMB
	switch r.DialectRevision() {
	case wire.SMB2, wire.UnknownSMB:
		return nil, invalidResponse(wire.SMB2_NEGOTIATE, "unexpected dialect returned")
	}

	if !slices.Contains(req.Dialects, Dialect(r.DialectRevision())) {
		return nil, invalidResponse(wire.SMB2_NEGOTIATE, "unexpected dialect returned")
	}

	// [MS-SMB2] 3.2.5.2: The client SHOULD disconnect the connection if the
	// size, in bytes, received in MaxTransactSize, MaxReadSize, or
	// MaxWriteSize is less than 65536.
	if r.MaxTransactSize() < maxSingleCreditPayloadSize || r.MaxReadSize() < maxSingleCreditPayloadSize || r.MaxWriteSize() < maxSingleCreditPayloadSize {
		return nil, invalidResponse(wire.SMB2_NEGOTIATE, "payload size below 64KB")
	}

	conn.requireSigning = d.RequireMessageSigning || r.SecurityMode()&wire.SMB2_NEGOTIATE_SIGNING_REQUIRED != 0
	conn.capabilities = r.Capabilities()
	conn.dialect = r.DialectRevision()
	conn.maxTransactSize = r.MaxTransactSize()
	conn.maxReadSize = r.MaxReadSize()
	conn.maxWriteSize = r.MaxWriteSize()

	if conn.dialect != wire.SMB311 {
		// SMB 3.0 and 3.0.2 use only AES-128-CCM. Keep plaintext
		// connections usable when the caller excludes that cipher.
		if (conn.dialect == wire.SMB300 || conn.dialect == wire.SMB302) &&
			(len(d.Ciphers) == 0 || slices.Contains(d.Ciphers, AES128CCM)) {
			conn.cipherId = wire.AES128CCM
		}
		return conn, nil
	}

	// handle context for SMB311
	var seenPreauth, seenEncryption, seenCompression, seenTransport bool
	list := r.Contexts()
	for count := r.NegotiateContextCount(); count > 0; count-- {
		nc := wire.NegotiateContextDecoder(list)
		if nc.IsInvalid() {
			return nil, invalidResponse(wire.SMB2_NEGOTIATE, "broken negotiate context format")
		}

		switch nc.ContextType() {
		case wire.SMB2_PREAUTH_INTEGRITY_CAPABILITIES:
			if seenPreauth {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "duplicate preauth integrity capabilities context")
			}
			seenPreauth = true

			data := wire.HashContextDataDecoder(nc.Data())
			if data.IsInvalid() {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "broken hash context data format")
			}

			algs := data.HashAlgorithms()

			if len(algs) != 1 {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "multiple hash algorithms")
			}

			if !slices.Contains(clientHashAlgorithms, algs[0]) {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "unsupported hash algorithm")
			}

			conn.preauthIntegrityHashId = algs[0]

			// Handshake requests are executed sequentially without concurrent access,
			// so conn.encodeBuf still holds the encoded request packet.
			updatePreauthHash(&conn.preauthIntegrityHashValue, conn.encodeBuf)
			updatePreauthHash(&conn.preauthIntegrityHashValue, res.bytes(0))
		case wire.SMB2_ENCRYPTION_CAPABILITIES:
			if seenEncryption {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "duplicate encryption capabilities context")
			}
			seenEncryption = true

			data := wire.CipherContextDataDecoder(nc.Data())
			if data.IsInvalid() {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "broken cipher context data format")
			}

			ciphs := data.Ciphers()

			if len(ciphs) != 1 {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "multiple cipher algorithms")
			}

			offeredCiphers := d.Ciphers
			if len(offeredCiphers) == 0 {
				offeredCiphers = clientCiphers
			}
			// [MS-SMB2] 3.2.5.2 permits Ciphers[0] == 0 to disable encryption;
			// zero is valid only as the server's selected value, not as a client offer.
			if ciphs[0] != 0 && !slices.Contains(offeredCiphers, ciphs[0]) {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "unsupported cipher algorithm")
			}

			conn.cipherId = uint16(ciphs[0])
		case wire.SMB2_TRANSPORT_CAPABILITIES:
			if seenTransport {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "duplicate transport capabilities context")
			}
			seenTransport = true
			data := wire.TransportContextDataDecoder(nc.Data())
			if data.IsInvalid() {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "broken transport context data format")
			}
			conn.acceptTransportSecurity = isQUIC && d.DisableEncryptionOverSecureTransport && data.Flags()&wire.SMB2_ACCEPT_TRANSPORT_LEVEL_SECURITY != 0
		case wire.SMB2_COMPRESSION_CAPABILITIES:
			if seenCompression {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "duplicate compression capabilities context")
			}
			seenCompression = true

			data := wire.CompressionContextDataDecoder(nc.Data())
			if data.IsInvalid() {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "broken compression context data format")
			}
			if data.Flags() != wire.SMB2_COMPRESSION_CAPABILITIES_FLAG_NONE && data.Flags() != wire.SMB2_COMPRESSION_CAPABILITIES_FLAG_CHAINED {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "invalid compression context flags")
			}

			algorithms := data.CompressionAlgorithms()
			if len(algorithms) == 0 {
				return nil, invalidResponse(wire.SMB2_NEGOTIATE, "no compression algorithms")
			}

			seenAlgorithms := make(map[uint16]struct{}, len(algorithms))
			for _, algorithm := range algorithms {
				if algorithm >= 32 {
					return nil, invalidResponse(wire.SMB2_NEGOTIATE, "invalid compression algorithm")
				}
				if _, ok := seenAlgorithms[algorithm]; ok {
					return nil, invalidResponse(wire.SMB2_NEGOTIATE, "duplicate compression algorithm")
				}
				seenAlgorithms[algorithm] = struct{}{}
			}

			if len(algorithms) == 1 && algorithms[0] == wire.SMB2_COMPRESSION_ALGORITHM_NONE {
				conn.compressionIds = nil
				break
			}

			for _, algorithm := range algorithms {
				if !slices.Contains(clientCompressionAlgorithms, algorithm) {
					return nil, invalidResponse(wire.SMB2_NEGOTIATE, "unsupported compression algorithm")
				}
			}
			conn.compressionIds = append([]uint16(nil), algorithms...)
			// The client offers unchained LZ4 only. A server's CHAINED bit
			// does not opt this connection into chained compression.
			conn.supportsChainedCompression = false
		default:
			// skip unsupported context
		}

		off := nc.Next()

		if len(list) < off {
			list = nil
		} else {
			list = list[off:]
		}
	}

	if !seenPreauth {
		return nil, invalidResponse(wire.SMB2_NEGOTIATE, "missing preauth integrity capabilities context")
	}

	return conn, nil
}

// makeNegotiateRequest builds the NEGOTIATE request from the Dialer.
func (d *Dialer) makeNegotiateRequest(dialects []Dialect, acceptTransportSecurity bool) (*wire.NegotiateRequest, error) {
	req := new(wire.NegotiateRequest)

	if d.RequireMessageSigning {
		req.SecurityMode = wire.SMB2_NEGOTIATE_SIGNING_REQUIRED
	} else {
		req.SecurityMode = wire.SMB2_NEGOTIATE_SIGNING_ENABLED
	}

	req.Capabilities = clientCapabilities

	if d.ClientGuid == uuid.Nil {
		req.ClientGuid = uuid.New()
	} else {
		req.ClientGuid = d.ClientGuid
	}

	req.Dialects = dialects

	hasSMB311 := slices.Contains(dialects, SMB311)
	hasSMB3 := false
	for _, dialect := range dialects {
		if SMB300 <= dialect {
			hasSMB3 = true
			break
		}
	}

	if !hasSMB3 {
		req.Capabilities = 0
	}

	if hasSMB311 {
		hc, err := newHashContext()
		if err != nil {
			return nil, err
		}
		req.Contexts = append(req.Contexts, hc, newCipherContext(d.Ciphers), newCompressionContext())
		if acceptTransportSecurity {
			req.Contexts = append(req.Contexts, &wire.TransportContext{Flags: wire.SMB2_ACCEPT_TRANSPORT_LEVEL_SECURITY})
		}
	}

	return req, nil
}
