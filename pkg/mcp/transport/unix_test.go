// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transport

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempSocketPath creates a short socket path suitable for macOS (max 104 chars).
// It uses /tmp directly instead of t.TempDir() which creates long paths.
func tempSocketPath(t *testing.T) string {
	t.Helper()
	// Use a short random suffix to avoid conflicts
	path := fmt.Sprintf("/tmp/mcp-test-%d.sock", rand.Int63())
	t.Cleanup(func() {
		os.Remove(path)
	})
	return path
}

// startAndWait starts the transport in a background goroutine and waits for
// the socket to be ready. Returns a cancel function to stop the transport.
func startAndWait(t *testing.T, transport *UnixTransport, handler MessageHandler, socketPath string) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	// Channel to capture any startup error
	errCh := make(chan error, 1)

	// Start the transport in background
	go func() {
		err := transport.Start(ctx, handler)
		if err != nil && err != context.Canceled {
			errCh <- err
		}
	}()

	// Wait for socket to be ready with extended timeout
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// Check for startup error
		select {
		case err := <-errCh:
			t.Fatalf("transport.Start failed: %v", err)
		default:
		}

		info, err := os.Stat(socketPath)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			// Socket file exists, try to connect
			conn, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
			if err == nil {
				conn.Close()
				return cancel
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Check one more time for error before failing
	select {
	case err := <-errCh:
		t.Fatalf("transport.Start failed: %v", err)
	default:
	}

	t.Fatalf("socket %s did not become ready", socketPath)
	return cancel
}

func TestNewUnixTransport(t *testing.T) {
	t.Run("applies default values", func(t *testing.T) {
		config := UnixConfig{
			Path: "/tmp/test.sock",
		}
		transport := NewUnixTransport(config)

		defaults := DefaultConfig()
		assert.Equal(t, defaults.MaxMessageSize, transport.config.MaxMessageSize)
		assert.Equal(t, defaults.ReadTimeout, transport.config.ReadTimeout)
		assert.Equal(t, defaults.WriteTimeout, transport.config.WriteTimeout)
		assert.Equal(t, defaults.MaxConnections, transport.config.MaxConnections)
		assert.Equal(t, defaults.ShutdownTimeout, transport.config.ShutdownTimeout)
		assert.Equal(t, uint32(0600), transport.config.Mode)
	})

	t.Run("preserves custom values", func(t *testing.T) {
		config := UnixConfig{
			TransportConfig: TransportConfig{
				MaxMessageSize:  1024,
				ReadTimeout:     10,
				WriteTimeout:    20,
				MaxConnections:  50,
				ShutdownTimeout: 15,
			},
			Path: "/tmp/test.sock",
			Mode: 0644,
		}
		transport := NewUnixTransport(config)

		assert.Equal(t, 1024, transport.config.MaxMessageSize)
		assert.Equal(t, 10, transport.config.ReadTimeout)
		assert.Equal(t, 20, transport.config.WriteTimeout)
		assert.Equal(t, 50, transport.config.MaxConnections)
		assert.Equal(t, 15, transport.config.ShutdownTimeout)
		assert.Equal(t, uint32(0644), transport.config.Mode)
	})

	t.Run("initializes internal state", func(t *testing.T) {
		transport := NewUnixTransport(UnixConfig{Path: "/tmp/test.sock"})

		assert.NotNil(t, transport.done)
		assert.NotNil(t, transport.conns)
		assert.False(t, transport.started)
	})
}

func TestUnixTransport_Type(t *testing.T) {
	transport := NewUnixTransport(UnixConfig{Path: "/tmp/test.sock"})
	assert.Equal(t, TransportTypeUnix, transport.Type())
}

func TestUnixTransport_Address(t *testing.T) {
	path := "/tmp/test.sock"
	transport := NewUnixTransport(UnixConfig{Path: path})
	assert.Equal(t, path, transport.Address())
}

func TestUnixTransport_ActiveConnections(t *testing.T) {
	transport := NewUnixTransport(UnixConfig{Path: "/tmp/test.sock"})

	assert.Equal(t, 0, transport.ActiveConnections())

	atomic.AddInt32(&transport.connCount, 1)
	assert.Equal(t, 1, transport.ActiveConnections())

	atomic.AddInt32(&transport.connCount, 2)
	assert.Equal(t, 3, transport.ActiveConnections())
}

func TestUnixTransport_StartAlreadyStarted(t *testing.T) {
	transport := NewUnixTransport(UnixConfig{Path: "/tmp/test.sock"})
	transport.started = true

	err := transport.Start(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestUnixTransport_StartAndStop(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  1,
			WriteTimeout: 1,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		return []byte(`{"result":"ok"}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	cancel()

	// Give it time to shut down
	time.Sleep(100 * time.Millisecond)

	// Socket should be removed after shutdown
	_, err := os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err), "socket file should be removed after shutdown")
}

func TestUnixTransport_RemoveExistingSocket(t *testing.T) {
	socketPath := tempSocketPath(t)

	// Create an existing file (not a socket)
	f, err := os.Create(socketPath)
	require.NoError(t, err)
	f.Close()

	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  1,
			WriteTimeout: 1,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		return nil, nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	// Verify the socket exists and is a socket
	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSocket != 0, "file should be a socket")
}

func TestUnixTransport_ClientCommunication(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  5,
			WriteTimeout: 5,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		assert.Equal(t, TransportTypeUnix, connInfo.Transport)
		assert.NotEmpty(t, connInfo.ID)
		return []byte(`{"jsonrpc":"2.0","result":"echo","id":1}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	// Connect as client
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send a message
	_, err = conn.Write([]byte(`{"jsonrpc":"2.0","method":"test","id":1}` + "\n"))
	require.NoError(t, err)

	// Read response
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	require.NoError(t, err)
	assert.Contains(t, string(buf[:n]), `"result":"echo"`)
}

func TestUnixTransport_MultipleClients(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:    5,
			WriteTimeout:   5,
			MaxConnections: 10,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	var messageCount int32
	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		atomic.AddInt32(&messageCount, 1)
		return []byte(`{"result":"ok"}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	// Connect multiple clients
	numClients := 5
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Logf("client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			msg := fmt.Sprintf(`{"id":%d,"method":"test"}`, clientID)
			_, err = conn.Write([]byte(msg + "\n"))
			if err != nil {
				t.Logf("client %d: write error: %v", clientID, err)
				return
			}

			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, err = conn.Read(buf)
			if err != nil {
				t.Logf("client %d: read error: %v", clientID, err)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int32(numClients), atomic.LoadInt32(&messageCount))
}

func TestUnixTransport_MaxConnections(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:    5,
			WriteTimeout:   5,
			MaxConnections: 2,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	// Handler that blocks to keep connections open
	handlerReady := make(chan struct{}, 10)
	handlerBlock := make(chan struct{})
	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		handlerReady <- struct{}{}
		<-handlerBlock
		return []byte(`{"result":"ok"}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	// Connect max connections
	conns := make([]net.Conn, 2)
	for i := 0; i < 2; i++ {
		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		conns[i] = conn

		// Send message to trigger handler
		_, err = conn.Write([]byte(`{"test":"msg"}` + "\n"))
		require.NoError(t, err)

		// Wait for handler to be ready
		select {
		case <-handlerReady:
		case <-time.After(2 * time.Second):
			t.Fatal("handler not ready in time")
		}
	}

	// Verify we have max connections
	assert.Equal(t, 2, transport.ActiveConnections())

	// Clean up
	close(handlerBlock)
	for _, conn := range conns {
		conn.Close()
	}
}

func TestUnixTransport_ConnectionHandler(t *testing.T) {
	socketPath := tempSocketPath(t)

	var connectCalled, disconnectCalled int32
	connHandler := &mockConnectionHandler{
		onConnect: func(ctx context.Context, conn Connection) error {
			atomic.AddInt32(&connectCalled, 1)
			assert.Equal(t, TransportTypeUnix, conn.Info().Transport)
			return nil
		},
		onDisconnect: func(ctx context.Context, conn Connection) {
			atomic.AddInt32(&disconnectCalled, 1)
		},
	}

	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:       5,
			WriteTimeout:      5,
			ConnectionHandler: connHandler,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		return []byte(`{"result":"ok"}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	// Connect client
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)

	// Send a message to ensure handler is called
	_, err = conn.Write([]byte(`{"test":"msg"}` + "\n"))
	require.NoError(t, err)

	// Read response
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.Read(buf)

	// Close client connection
	conn.Close()

	// Wait for disconnect handler
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&disconnectCalled) > 0
	}, 2*time.Second, 10*time.Millisecond)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&connectCalled), int32(1))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&disconnectCalled), int32(1))
}

func TestUnixTransport_ConnectionHandlerReject(t *testing.T) {
	socketPath := tempSocketPath(t)

	connHandler := &mockConnectionHandler{
		onConnect: func(ctx context.Context, conn Connection) error {
			return fmt.Errorf("connection rejected")
		},
		onDisconnect: func(ctx context.Context, conn Connection) {},
	}

	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:       5,
			WriteTimeout:      5,
			ConnectionHandler: connHandler,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	var handlerCalled int32
	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		atomic.AddInt32(&handlerCalled, 1)
		return []byte(`{"result":"ok"}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	// Connect client
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)

	// Send a message
	_, err = conn.Write([]byte(`{"test":"msg"}` + "\n"))
	require.NoError(t, err)

	// Connection should be closed without processing message
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&handlerCalled))

	conn.Close()
}

func TestUnixTransport_EmptyMessage(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  5,
			WriteTimeout: 5,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	var messageCount int32
	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		atomic.AddInt32(&messageCount, 1)
		return []byte(`{"result":"ok"}`), nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send empty lines (should be ignored)
	_, err = conn.Write([]byte("\n\n\n"))
	require.NoError(t, err)

	// Send actual message
	_, err = conn.Write([]byte(`{"test":"msg"}` + "\n"))
	require.NoError(t, err)

	// Wait for message processing
	time.Sleep(100 * time.Millisecond)

	// Only the non-empty message should be processed
	assert.Equal(t, int32(1), atomic.LoadInt32(&messageCount))
}

func TestUnixTransport_NilResponse(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  5,
			WriteTimeout: 5,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		// Return nil response (notification)
		return nil, nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send message
	_, err = conn.Write([]byte(`{"test":"notification"}` + "\n"))
	require.NoError(t, err)

	// No response should be written - verify with timeout
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	assert.Error(t, err) // Should timeout or EOF
}

func TestUnixTransport_HandlerError(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  5,
			WriteTimeout: 5,
		},
		Path:           socketPath,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		return nil, fmt.Errorf("handler error")
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send message that will cause handler error
	_, err = conn.Write([]byte(`{"test":"error"}` + "\n"))
	require.NoError(t, err)

	// Connection should still be alive for subsequent messages
	_, err = conn.Write([]byte(`{"test":"second"}` + "\n"))
	require.NoError(t, err)
}

func TestUnixTransport_StopWithoutStart(t *testing.T) {
	transport := NewUnixTransport(UnixConfig{Path: "/tmp/test.sock"})

	// Stop without starting should not error
	err := transport.Stop(context.Background())
	assert.NoError(t, err)
}

func TestUnixTransport_SocketPermissions(t *testing.T) {
	socketPath := tempSocketPath(t)
	config := UnixConfig{
		TransportConfig: TransportConfig{
			ReadTimeout:  1,
			WriteTimeout: 1,
		},
		Path:           socketPath,
		Mode:           0660,
		RemoveExisting: true,
	}
	transport := NewUnixTransport(config)

	handler := MessageHandlerFunc(func(ctx context.Context, message []byte, connInfo ConnectionInfo) ([]byte, error) {
		return nil, nil
	})

	cancel := startAndWait(t, transport, handler, socketPath)
	defer cancel()

	info, err := os.Stat(socketPath)
	require.NoError(t, err)

	// Check that permissions match (masking with 0777 to ignore socket type bits)
	assert.Equal(t, os.FileMode(0660), info.Mode().Perm())
}

func TestUnixConn_Info(t *testing.T) {
	mockConn := &mockNetConn{}
	connInfo := ConnectionInfo{
		ID:          "test-id",
		RemoteAddr:  "test-addr",
		Transport:   TransportTypeUnix,
		ConnectedAt: time.Now(),
	}

	uc := &unixConn{
		Conn:     mockConn,
		connInfo: connInfo,
	}

	info := uc.Info()
	assert.Equal(t, "test-id", info.ID)
	assert.Equal(t, "test-addr", info.RemoteAddr)
	assert.Equal(t, TransportTypeUnix, info.Transport)
}

// mockConnectionHandler implements ConnectionHandler for testing
type mockConnectionHandler struct {
	onConnect    func(ctx context.Context, conn Connection) error
	onDisconnect func(ctx context.Context, conn Connection)
}

func (m *mockConnectionHandler) OnConnect(ctx context.Context, conn Connection) error {
	if m.onConnect != nil {
		return m.onConnect(ctx, conn)
	}
	return nil
}

func (m *mockConnectionHandler) OnDisconnect(ctx context.Context, conn Connection) {
	if m.onDisconnect != nil {
		m.onDisconnect(ctx, conn)
	}
}

// mockNetConn implements net.Conn for testing
type mockNetConn struct {
	net.Conn
}

func (m *mockNetConn) RemoteAddr() net.Addr {
	return &net.UnixAddr{Name: "mock", Net: "unix"}
}

func (m *mockNetConn) Close() error {
	return nil
}
