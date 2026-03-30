// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcecho

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// tcpPort defines the port the RC backend is listening on for a TCP ping/pong
// test.
const tcpPort = 8042

// maxMsgSize defines the maximum allowed payload size for TCP ping / pong
// payload.
const maxMsgSize = uint16(8 * 1024)

// TCPPingPonger performs a ping/pong test by satisfying the PingPonger
// interface using a raw TCP connection and length-prefixed framing.
//
// Frame structure:
//
//	[length prefix][payload ...]
//
// Where `length prefix` is a u16 (2 byte, unsigned) which specifies the length
// of payload in bytes. Lengths values are little endian.
type TCPPingPonger struct {
	conn net.Conn
	buf  []byte
}

func NewTCPPingPonger(ctx context.Context, httpClient *api.HTTPClient) (*TCPPingPonger, error) {
	conn, err := newTCPClient(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	return &TCPPingPonger{
		conn: conn,
		buf:  make([]byte, 0, maxMsgSize),
	}, nil
}

func (g *TCPPingPonger) Recv(_ context.Context) ([]byte, error) {
	g.bumpTimeout() // timeout for entire payload read.

	// Read the length prefix into an ad-hoc buffer so the main buffer is
	// reserved exclusively for payload data, suitable for sharing with the
	// caller.
	var hdr [2]byte
	_, err := io.ReadFull(g.conn, hdr[:])
	if err != nil {
		return nil, err
	}

	// Read the LE encoded length value.
	wantLen := binary.LittleEndian.Uint16(hdr[:])
	// Refuse to read a giant message.
	if wantLen > maxMsgSize {
		return nil, errors.New("recv frame exceeds max message size")
	}

	// Read the payload, which is exactly wantLen number of bytes long.
	g.resetBuffer()
	_, err = io.ReadFull(g.conn, g.buf[:wantLen])
	if err != nil {
		return nil, err
	}

	return g.buf[:wantLen], nil
}

func (g *TCPPingPonger) Send(_ context.Context, data []byte) error {
	g.bumpTimeout()

	if len(data) > math.MaxUint16 {
		return errors.New("data size too large for send")
	}
	dataLen := uint16(len(data)) // Safe cast due to above check

	// Write the length prefix from an ad-hoc buffer so the main buffer
	// (which may back the data slice from a prior Recv) is not overwritten.
	var hdr [2]byte
	binary.LittleEndian.PutUint16(hdr[:], dataLen)
	_, err := g.conn.Write(hdr[:])
	if err != nil {
		return err
	}

	// Write docs state the impl is required to transmit the full buffer before
	// returning (unlike Read which can return partial data).
	_, err = g.conn.Write(data)
	return err
}

func (g *TCPPingPonger) GracefulClose() {
	g.conn.Close()
}

func (g *TCPPingPonger) bumpTimeout() {
	_ = g.conn.SetDeadline(time.Now().Add(5 * time.Minute))
}

func (g *TCPPingPonger) resetBuffer() {
	g.buf = g.buf[:0]
}

// newTCPClient connects to the RC TCP backend and returns a new connection or a
// connection / handshake error.
//
// The "endpointPath" specifies the resource path to connect to, which is
// appended to the client baseURL.
func newTCPClient(ctx context.Context, httpClient *api.HTTPClient) (net.Conn, error) {
	// Parse the "base URL" the client uses to connect to RC.
	url, err := httpClient.BaseURL()
	if err != nil {
		return nil, err
	}
	// Construct the dial address using only the hostname (stripping any
	// port in the base URL) so that "host:443" doesn't produce the invalid
	// address "host:443:8042".
	endpoint := net.JoinHostPort(url.Hostname(), strconv.Itoa(tcpPort))

	log.Debugf("connecting to tcp endpoint %s", endpoint)

	// Extract the TLS config from the HTTP client.
	transport, err := httpClient.Transport()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Dial the connection and perform a TLS handshake.
	dialer := tls.Dialer{
		Config: transport.TLSClientConfig,
	}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, err
	}

	log.Debug("tcp endpoint connected")

	return conn, nil
}
