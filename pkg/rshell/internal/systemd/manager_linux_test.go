// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package systemd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testManagerMachineID = "0123456789abcdef0123456789abcdef"
	testManagerBusGUID   = "fedcba9876543210fedcba9876543210"
)

func TestDialManagerBusConnectsToRealSocket(t *testing.T) {
	dir := managerLinuxTestDir(t)
	path := filepath.Join(dir, "bus")
	listener := listenUnixSocket(t, path)

	connection, err := (&Client{}).dialManagerBus(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })

	require.NoError(t, listener.SetDeadline(time.Now().Add(time.Second)))
	accepted, err := listener.AcceptUnix()
	require.NoError(t, err)
	t.Cleanup(func() { _ = accepted.Close() })

	require.NoError(t, connection.SetDeadline(time.Now().Add(time.Second)))
	require.NoError(t, accepted.SetDeadline(time.Now().Add(time.Second)))
	_, err = connection.Write([]byte{0x42})
	require.NoError(t, err)
	var received [1]byte
	_, err = io.ReadFull(accepted, received[:])
	require.NoError(t, err)
	assert.Equal(t, byte(0x42), received[0])
}

func TestDialManagerBusRejectsNonSocketEndpoints(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string) string
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "file")
				require.NoError(t, os.WriteFile(path, []byte("not a socket"), 0o600))
				return path
			},
		},
		{
			name: "final symlink to live socket",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "real")
				listenUnixSocket(t, target)
				path := filepath.Join(dir, "link")
				require.NoError(t, os.Symlink(target, path))
				return path
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.setup(t, managerLinuxTestDir(t))
			connection, err := (&Client{}).dialManagerBus(context.Background(), path)
			assert.Nil(t, connection)
			require.EqualError(t, err, "systemd manager bus endpoint is not a Unix socket")
		})
	}
}

func TestDialPinnedManagerBusIgnoresPathReplacementAndClosesPin(t *testing.T) {
	dir := managerLinuxTestDir(t)
	path := filepath.Join(dir, "bus")
	original := listenUnixSocket(t, path)

	pinned, err := (&Client{}).openManagerBusSocket(path)
	require.NoError(t, err)
	require.NoError(t, os.Rename(path, filepath.Join(dir, "original")))
	attacker := listenUnixSocket(t, path)

	connection, dialErr := dialPinnedManagerBus(context.Background(), pinned)
	_, statErr := pinned.Stat()
	require.ErrorIs(t, statErr, os.ErrClosed)
	require.NoError(t, dialErr)
	t.Cleanup(func() { _ = connection.Close() })

	require.NoError(t, original.SetDeadline(time.Now().Add(time.Second)))
	accepted, err := original.AcceptUnix()
	require.NoError(t, err)
	t.Cleanup(func() { _ = accepted.Close() })

	require.NoError(t, connection.SetDeadline(time.Now().Add(time.Second)))
	require.NoError(t, accepted.SetDeadline(time.Now().Add(time.Second)))
	_, err = connection.Write([]byte{0x7f})
	require.NoError(t, err)
	var received [1]byte
	_, err = io.ReadFull(accepted, received[:])
	require.NoError(t, err)
	assert.Equal(t, byte(0x7f), received[0])

	require.NoError(t, attacker.SetDeadline(time.Now().Add(100*time.Millisecond)))
	_, err = attacker.AcceptUnix()
	require.Error(t, err)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout())
}

func TestDialPinnedManagerBusHonorsDeadlineAndClosesPin(t *testing.T) {
	dir := managerLinuxTestDir(t)
	path := filepath.Join(dir, "bus")
	listenUnixSocket(t, path)
	pinned, err := (&Client{}).openManagerBusSocket(path)
	require.NoError(t, err)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	connection, err := dialPinnedManagerBus(ctx, pinned)
	assert.Nil(t, connection)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, statErr := pinned.Stat()
	require.ErrorIs(t, statErr, os.ErrClosed)
}

func TestOpenManagerBusAuthenticatesAndVerifiesMachineID(t *testing.T) {
	tests := []struct {
		name          string
		peerMachineID string
		wantErr       string
	}{
		{name: "matching machine ID", peerMachineID: testManagerMachineID},
		{
			name:          "mismatching machine ID",
			peerMachineID: "11111111111111111111111111111111",
			wantErr:       "systemd manager peer machine ID does not match the configured target",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := managerLinuxTestDir(t)
			path := filepath.Join(dir, "bus")
			listener := listenUnixSocket(t, path)
			serverDone := startRawManagerDBusServer(listener, test.peerMachineID)
			client := managerLinuxTestClient(t, dir, path)

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			bus, openErr := client.openManagerBus(ctx)
			var closeErr error
			if bus != nil {
				closeErr = bus.connection.Close()
			}
			serverErr := waitManagerDBusServer(t, serverDone)

			require.NoError(t, serverErr)
			require.NoError(t, closeErr)
			if test.wantErr == "" {
				require.NoError(t, openErr)
				require.NotNil(t, bus)
				return
			}
			assert.Nil(t, bus)
			require.EqualError(t, openErr, test.wantErr)
		})
	}
}

func TestOpenManagerBusHonorsDeadlineDuringAuthentication(t *testing.T) {
	dir := managerLinuxTestDir(t)
	path := filepath.Join(dir, "bus")
	listener := listenUnixSocket(t, path)
	authStarted := make(chan struct{})
	serverDone := startStalledManagerAuthServer(listener, authStarted)
	client := managerLinuxTestClient(t, dir, path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	bus, err := client.openManagerBus(ctx)
	assert.Nil(t, bus)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-authStarted:
	default:
		t.Fatal("manager D-Bus authentication did not start before the deadline")
	}
	require.NoError(t, waitManagerDBusServer(t, serverDone))
}

func managerLinuxTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rsd-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func managerLinuxTestClient(t *testing.T, dir, socketPath string) *Client {
	t.Helper()
	machineIDPath := filepath.Join(dir, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testManagerMachineID+"\n"), 0o600))
	return NewClient(Target{MachineIDPath: machineIDPath, ManagerBusSocket: socketPath})
}

func startRawManagerDBusServer(listener *net.UnixListener, machineID string) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- serveRawManagerDBus(listener, machineID)
	}()
	return done
}

func serveRawManagerDBus(listener *net.UnixListener, machineID string) error {
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	connection, err := listener.AcceptUnix()
	if err != nil {
		return fmt.Errorf("accept manager D-Bus connection: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	reader := bufio.NewReader(connection)
	if err := authenticateRawManagerDBus(reader, connection); err != nil {
		return err
	}

	hello, err := dbus.DecodeMessage(reader)
	if err != nil {
		return fmt.Errorf("decode Hello call: %w", err)
	}
	if err := validateRawManagerCall(hello, 0, "org.freedesktop.DBus", dbus.ObjectPath("/org/freedesktop/DBus"), "org.freedesktop.DBus", "Hello"); err != nil {
		return fmt.Errorf("validate Hello call: %w", err)
	}
	if err := writeRawManagerReply(connection, hello, 1, ":1.42"); err != nil {
		return fmt.Errorf("reply to Hello call: %w", err)
	}

	machineIDCall, err := dbus.DecodeMessage(reader)
	if err != nil {
		return fmt.Errorf("decode Peer.GetMachineId call: %w", err)
	}
	if err := validateRawManagerCall(machineIDCall, dbus.FlagNoAutoStart, systemdBusDestination, systemdManagerPath, "org.freedesktop.DBus.Peer", "GetMachineId"); err != nil {
		return fmt.Errorf("validate Peer.GetMachineId call: %w", err)
	}
	if err := writeRawManagerReply(connection, machineIDCall, 2, machineID); err != nil {
		return fmt.Errorf("reply to Peer.GetMachineId call: %w", err)
	}

	var trailing [1]byte
	_, err = reader.Read(trailing[:])
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("wait for manager D-Bus client cleanup: %w", err)
	}
	return fmt.Errorf("manager D-Bus client sent unexpected data before closing")
}

func authenticateRawManagerDBus(reader *bufio.Reader, writer io.Writer) error {
	if err := readInitialManagerAuth(reader); err != nil {
		return err
	}
	if err := writeAll(writer, []byte("REJECTED EXTERNAL\r\n")); err != nil {
		return fmt.Errorf("offer EXTERNAL authentication: %w", err)
	}
	line, err := readManagerAuthLine(reader)
	if err != nil {
		return err
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || len(fields) > 3 || fields[0] != "AUTH" || fields[1] != "EXTERNAL" {
		return fmt.Errorf("unexpected manager D-Bus authentication request %q", line)
	}
	if len(fields) == 3 {
		identity, err := hex.DecodeString(fields[2])
		if err != nil || string(identity) != strconv.Itoa(os.Geteuid()) {
			return fmt.Errorf("manager D-Bus EXTERNAL authentication used an unexpected identity")
		}
	}
	if err := writeAll(writer, []byte("OK "+testManagerBusGUID+"\r\n")); err != nil {
		return fmt.Errorf("accept EXTERNAL authentication: %w", err)
	}
	line, err = readManagerAuthLine(reader)
	if err != nil {
		return err
	}
	if line != "BEGIN" {
		return fmt.Errorf("unexpected manager D-Bus authentication terminator %q", line)
	}
	return nil
}

func readInitialManagerAuth(reader *bufio.Reader) error {
	initial, err := reader.ReadByte()
	if err != nil {
		return fmt.Errorf("read manager D-Bus authentication prefix: %w", err)
	}
	if initial != 0 {
		return fmt.Errorf("manager D-Bus authentication omitted the initial NUL byte")
	}
	line, err := readManagerAuthLine(reader)
	if err != nil {
		return err
	}
	if line != "AUTH" {
		return fmt.Errorf("unexpected initial manager D-Bus authentication request %q", line)
	}
	return nil
}

func readManagerAuthLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read manager D-Bus authentication line: %w", err)
	}
	if !strings.HasSuffix(line, "\r\n") {
		return "", fmt.Errorf("manager D-Bus authentication line is not CRLF terminated")
	}
	return strings.TrimSuffix(line, "\r\n"), nil
}

func validateRawManagerCall(message *dbus.Message, flags dbus.Flags, destination string, path dbus.ObjectPath, iface, member string) error {
	if message.Type != dbus.TypeMethodCall {
		return fmt.Errorf("message type is %s, not method call", message.Type)
	}
	if message.Flags != flags {
		return fmt.Errorf("message flags are %v, expected %v", message.Flags, flags)
	}
	if message.Serial() == 0 {
		return fmt.Errorf("message serial is zero")
	}
	if len(message.Body) != 0 {
		return fmt.Errorf("message body is not empty")
	}
	expected := map[dbus.HeaderField]any{
		dbus.FieldDestination: destination,
		dbus.FieldPath:        path,
		dbus.FieldInterface:   iface,
		dbus.FieldMember:      member,
	}
	for field, want := range expected {
		value, ok := message.Headers[field]
		if !ok {
			return fmt.Errorf("message header %d is missing", field)
		}
		if !reflect.DeepEqual(value.Value(), want) {
			return fmt.Errorf("message header %d is %v, expected %v", field, value.Value(), want)
		}
	}
	return nil
}

func writeRawManagerReply(writer io.Writer, request *dbus.Message, serial uint32, body ...any) error {
	headers := map[dbus.HeaderField]dbus.Variant{
		dbus.FieldReplySerial: dbus.MakeVariant(request.Serial()),
	}
	if len(body) > 0 {
		headers[dbus.FieldSignature] = dbus.MakeVariant(dbus.SignatureOf(body...))
	}
	reply := &dbus.Message{Type: dbus.TypeMethodReply, Headers: headers, Body: body}
	var encoded bytes.Buffer
	if err := reply.EncodeTo(&encoded, binary.LittleEndian); err != nil {
		return err
	}
	frame := encoded.Bytes()
	if len(frame) < 12 {
		return fmt.Errorf("encoded manager D-Bus reply is truncated")
	}
	binary.LittleEndian.PutUint32(frame[8:12], serial)
	return writeAll(writer, frame)
}

func startStalledManagerAuthServer(listener *net.UnixListener, authStarted chan<- struct{}) <-chan error {
	done := make(chan error, 1)
	go func() {
		if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			done <- err
			return
		}
		connection, err := listener.AcceptUnix()
		if err != nil {
			done <- fmt.Errorf("accept stalled manager D-Bus connection: %w", err)
			return
		}
		defer connection.Close()
		if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			done <- err
			return
		}
		reader := bufio.NewReader(connection)
		if err := readInitialManagerAuth(reader); err != nil {
			done <- err
			return
		}
		close(authStarted)
		var trailing [1]byte
		_, err = reader.Read(trailing[:])
		if errors.Is(err, io.EOF) {
			done <- nil
			return
		}
		if err != nil {
			done <- fmt.Errorf("wait for stalled manager D-Bus client cleanup: %w", err)
			return
		}
		done <- fmt.Errorf("stalled manager D-Bus client sent unexpected data")
	}()
	return done
}

func waitManagerDBusServer(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(6 * time.Second):
		t.Fatal("timed out waiting for manager D-Bus test server")
		return nil
	}
}
