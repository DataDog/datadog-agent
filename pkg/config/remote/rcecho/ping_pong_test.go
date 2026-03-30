// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcecho

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// testPeer represents the server side of a PingPonger connection, providing
// helpers to send data to the client, read data from the client, and tear down
// the connection.
type testPeer struct {
	sendToClient   func(data []byte) error
	recvFromClient func() ([]byte, error)
	close          func()
}

// mockStream implements grpc.BidiStreamingClient[RunEchoTestRequest, RunEchoTestResponse]
// using channels for deterministic testing.
type mockStream struct {
	recvCh chan mockRecvMsg
	sendCh chan []byte
	ctx    context.Context
}

type mockRecvMsg struct {
	data []byte
	err  error
}

func (m *mockStream) Send(req *pbgo.RunEchoTestRequest) error {
	select {
	case m.sendCh <- req.Data:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *mockStream) Recv() (*pbgo.RunEchoTestResponse, error) {
	msg, ok := <-m.recvCh
	if !ok {
		return nil, io.EOF
	}
	if msg.err != nil {
		return nil, msg.err
	}
	return &pbgo.RunEchoTestResponse{Data: msg.data}, nil
}

func (m *mockStream) CloseSend() error             { return nil }
func (m *mockStream) Header() (metadata.MD, error) { return nil, nil }
func (m *mockStream) Trailer() metadata.MD         { return nil }
func (m *mockStream) Context() context.Context     { return m.ctx }
func (m *mockStream) SendMsg(_m any) error         { return nil }
func (m *mockStream) RecvMsg(_m any) error         { return nil }

// protocol defines a named factory that constructs a PingPonger and its
// corresponding testPeer for use in table-driven tests.
type protocol struct {
	name  string
	setup func(t *testing.T) (PingPonger, *testPeer)
}

func setupTCPPair(t *testing.T) (PingPonger, *testPeer) {
	// Use a real TCP listener so kernel buffers allow non-blocking writes
	// (net.Pipe is fully synchronous and deadlocks the test).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	serverCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverCh <- conn
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { clientConn.Close() })

	serverConn := <-serverCh
	t.Cleanup(func() { serverConn.Close() })

	pp := &TCPPingPonger{
		conn: clientConn,
		buf:  make([]byte, 0, maxMsgSize),
	}

	peer := &testPeer{
		sendToClient: func(data []byte) error {
			hdr := make([]byte, 2)
			binary.LittleEndian.PutUint16(hdr, uint16(len(data)))
			if _, err := serverConn.Write(hdr); err != nil {
				return err
			}
			_, err := serverConn.Write(data)
			return err
		},
		recvFromClient: func() ([]byte, error) {
			hdr := make([]byte, 2)
			if _, err := io.ReadFull(serverConn, hdr); err != nil {
				return nil, err
			}
			n := binary.LittleEndian.Uint16(hdr)
			buf := make([]byte, n)
			if _, err := io.ReadFull(serverConn, buf); err != nil {
				return nil, err
			}
			return buf, nil
		},
		close: func() { serverConn.Close() },
	}

	return pp, peer
}

func setupGrpcPair(t *testing.T) (PingPonger, *testPeer) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	recvCh := make(chan mockRecvMsg, 16)
	sendCh := make(chan []byte, 16)

	stream := &mockStream{
		recvCh: recvCh,
		sendCh: sendCh,
		ctx:    ctx,
	}

	pp := &GrpcPingPonger{
		stream: stream,
		cancel: cancel,
	}

	peer := &testPeer{
		sendToClient: func(data []byte) error {
			cp := make([]byte, len(data))
			copy(cp, data)
			recvCh <- mockRecvMsg{data: cp}
			return nil
		},
		recvFromClient: func() ([]byte, error) {
			select {
			case data := <-sendCh:
				return data, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		close: func() { close(recvCh) },
	}

	return pp, peer
}

func TestPingPonger(t *testing.T) {
	protocols := []protocol{
		{"tcp", setupTCPPair},
		{"grpc", setupGrpcPair},
	}

	tests := []struct {
		name string
		run  func(t *testing.T, client PingPonger, peer *testPeer)
	}{
		{
			name: "single_echo",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()
				payload := []byte("hello")

				require.NoError(t, peer.sendToClient(payload))

				got, err := client.Recv(ctx)
				require.NoError(t, err)
				assert.Equal(t, payload, got)

				require.NoError(t, client.Send(ctx, got))

				echoed, err := peer.recvFromClient()
				require.NoError(t, err)
				assert.Equal(t, payload, echoed)
			},
		},
		{
			name: "multiple_echoes",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()
				payloads := [][]byte{
					[]byte("one"),
					[]byte("two"),
					[]byte("three"),
					{0xDE, 0xAD, 0xBE, 0xEF},
				}

				for _, payload := range payloads {
					require.NoError(t, peer.sendToClient(payload))

					got, err := client.Recv(ctx)
					require.NoError(t, err)
					assert.Equal(t, payload, got)

					require.NoError(t, client.Send(ctx, got))

					echoed, err := peer.recvFromClient()
					require.NoError(t, err)
					assert.Equal(t, payload, echoed)
				}
			},
		},
		{
			name: "empty_payload",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()
				payload := []byte{}

				require.NoError(t, peer.sendToClient(payload))

				got, err := client.Recv(ctx)
				require.NoError(t, err)
				assert.Empty(t, got)

				require.NoError(t, client.Send(ctx, got))

				echoed, err := peer.recvFromClient()
				require.NoError(t, err)
				assert.Empty(t, echoed)
			},
		},
		{
			name: "large_payload",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()
				payload := bytes.Repeat([]byte("A"), int(maxMsgSize)-1)

				require.NoError(t, peer.sendToClient(payload))

				got, err := client.Recv(ctx)
				require.NoError(t, err)
				assert.Equal(t, payload, got)

				require.NoError(t, client.Send(ctx, got))

				echoed, err := peer.recvFromClient()
				require.NoError(t, err)
				assert.Equal(t, payload, echoed)
			},
		},
		{
			name: "max_size_payload",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()
				payload := bytes.Repeat([]byte("B"), int(maxMsgSize))

				require.NoError(t, peer.sendToClient(payload))

				got, err := client.Recv(ctx)
				require.NoError(t, err)
				assert.Equal(t, payload, got)

				require.NoError(t, client.Send(ctx, got))

				echoed, err := peer.recvFromClient()
				require.NoError(t, err)
				assert.Equal(t, payload, echoed)
			},
		},
		{
			name: "recv_after_peer_closes",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()
				peer.close()

				_, err := client.Recv(ctx)
				assert.Error(t, err)
			},
		},
		{
			name: "recv_error_after_successful_exchange",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				ctx := t.Context()

				// First exchange succeeds.
				require.NoError(t, peer.sendToClient([]byte("ok")))
				got, err := client.Recv(ctx)
				require.NoError(t, err)
				assert.Equal(t, []byte("ok"), got)
				require.NoError(t, client.Send(ctx, got))
				_, err = peer.recvFromClient()
				require.NoError(t, err)

				// Peer disconnects, next recv fails.
				peer.close()
				_, err = client.Recv(ctx)
				assert.Error(t, err)
			},
		},
		{
			name: "run_ping_pong_loop",
			run: func(t *testing.T, client PingPonger, peer *testPeer) {
				payloads := [][]byte{
					[]byte("ping1"),
					[]byte("ping2"),
					[]byte("ping3"),
				}

				// Run the server side in a goroutine: send payloads,
				// read echoes, then close.
				errCh := make(chan error, 1)
				go func() {
					defer peer.close()
					for _, p := range payloads {
						if err := peer.sendToClient(p); err != nil {
							errCh <- err
							return
						}
						got, err := peer.recvFromClient()
						if err != nil {
							errCh <- err
							return
						}
						if !bytes.Equal(p, got) {
							errCh <- errors.New("echo mismatch")
							return
						}
					}
					errCh <- nil
				}()

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				n, err := runPingPong(ctx, client)
				// runPingPong returns an error when the peer closes
				// (EOF or pipe closed).
				assert.Error(t, err)
				assert.Equal(t, uint(len(payloads)), n)

				// Server goroutine must complete without error.
				require.NoError(t, <-errCh)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, proto := range protocols {
				t.Run(proto.name, func(t *testing.T) {
					client, peer := proto.setup(t)
					tt.run(t, client, peer)
				})
			}
		})
	}
}

// TestTCPPingPonger_OversizedFrame verifies that the TCP framing rejects
// frames whose length prefix exceeds maxMsgSize.
func TestTCPPingPonger_OversizedFrame(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	serverCh := make(chan net.Conn, 1)
	go func() {
		conn, _ := ln.Accept()
		serverCh <- conn
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { clientConn.Close() })

	serverConn := <-serverCh
	t.Cleanup(func() { serverConn.Close() })

	pp := &TCPPingPonger{
		conn: clientConn,
		buf:  make([]byte, 0, maxMsgSize),
	}

	// Write a length prefix that exceeds the max.
	hdr := make([]byte, 2)
	binary.LittleEndian.PutUint16(hdr, uint16(maxMsgSize)+1)
	_, err = serverConn.Write(hdr)
	require.NoError(t, err)

	_, err = pp.Recv(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recv frame exceeds max message size")
}
