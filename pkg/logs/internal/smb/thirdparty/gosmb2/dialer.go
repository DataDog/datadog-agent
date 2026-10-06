package smb2

import (
	"context"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// Dialect represents an SMB dialect revision.
type Dialect = wire.Dialect

const (
	SMB202 Dialect = wire.SMB202
	SMB210 Dialect = wire.SMB210
	SMB300 Dialect = wire.SMB300
	SMB302 Dialect = wire.SMB302
	SMB311 Dialect = wire.SMB311
)

// Cipher represents an SMB 3.x encryption cipher algorithm ID.
type Cipher = wire.Cipher

const (
	AES128CCM Cipher = wire.AES128CCM
	AES128GCM Cipher = wire.AES128GCM
	AES256CCM Cipher = wire.AES256CCM
	AES256GCM Cipher = wire.AES256GCM
)

// Dialer configures independent SMB sessions. A Dialer may be used by
// concurrent callers; callers must not modify it or referenced configuration
// while it is in use.
type Dialer struct {
	Credentials Credentials
	// TransportDialer creates the transport for a server. If nil, TCPDialer{}
	// is used.
	TransportDialer  TransportDialer
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
	// DisableAAPLExtension skips negotiation of the AAPL extension when mounting
	// disk shares. The extension is required to manage security descriptors on
	// macOS SMBX. Disable it if you do not connect to macOS SMBX or do not need
	// to manage security descriptors there.
	DisableAAPLExtension bool
}

// Dial establishes a new authenticated session and transfers ownership of its
// connection to the returned Session.
func (d *Dialer) Dial(ctx context.Context, serverName string) (*Session, error) {
	if ctx == nil {
		panic("nil context")
	}
	if d == nil {
		panic("smb2: nil Dialer")
	}
	if serverName == "" {
		panic("smb2: empty server name")
	}
	if d.Credentials == nil {
		panic("smb2: missing credentials")
	}
	initiator, err := d.Credentials.NewInitiator(ctx, serverName)
	if err != nil {
		return nil, err
	}
	if initiator == nil {
		panic("smb2: Credentials returned a nil Initiator")
	}
	td := d.TransportDialer
	if td == nil {
		td = TCPDialer{}
	}
	transport, err := td.Dial(ctx, serverName)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		panic("smb2: TransportDialer returned nil")
	}
	dialer := protocol.Dialer{
		MaxCreditBalance:                     d.MaxCreditBalance,
		IOPipelineDepth:                      d.IOPipelineDepth,
		RequireMessageSigning:                d.RequireMessageSigning,
		ClientGuid:                           d.ClientGuid,
		SpecifiedDialects:                    d.SpecifiedDialects,
		Ciphers:                              d.Ciphers,
		DisableEncryptionOverSecureTransport: d.DisableEncryptionOverSecureTransport,
	}
	session, err := dialer.Dial(ctx, initiator, transport)
	if err != nil {
		return nil, err
	}
	return &Session{s: session, addr: serverName, disableAAPLExtension: d.DisableAAPLExtension}, nil
}
