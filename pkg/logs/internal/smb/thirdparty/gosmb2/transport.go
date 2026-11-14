package smb2

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
)

// Transport is a built-in SMB transport. Close is idempotent and unblocks I/O.
type Transport interface{ protocol.Transport }

// NewTransport applies SMB Direct TCP framing to conn.
func NewTransport(conn net.Conn) Transport { return protocol.NewTransport(conn) }

// TransportDialer establishes a transport for an SMB server.
// TransportDialer implementations must be safe for concurrent use.
type TransportDialer interface {
	Dial(ctx context.Context, serverName string) (Transport, error)
}

func resolveServerAddr(serverName string, defaultPort int) string {
	if _, _, err := net.SplitHostPort(serverName); err == nil {
		return serverName
	}
	return net.JoinHostPort(serverName, strconv.Itoa(defaultPort))
}

// TCPDialer establishes Direct TCP transports.
type TCPDialer struct {
	Port   int // 0 indicates port 445
	Dialer *net.Dialer
}

// Dial connects to serverName over TCP on the configured port.
func (d TCPDialer) Dial(ctx context.Context, serverName string) (Transport, error) {
	if ctx == nil {
		panic("nil context")
	}
	if d.Port < 0 {
		return nil, errors.New("smb2: invalid TCP port")
	}
	port := d.Port
	if port == 0 {
		port = 445
	}
	dialer := d.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	conn, err := dialer.DialContext(ctx, "tcp", resolveServerAddr(serverName, port))
	if err != nil {
		return nil, err
	}
	return NewTransport(conn), nil
}

var (
	_ TransportDialer = TCPDialer{}
)
