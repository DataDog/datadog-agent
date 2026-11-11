// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryReadWriteCloser struct {
	reader *bytes.Reader
	writes bytes.Buffer
}

func (m *memoryReadWriteCloser) Read(output []byte) (int, error) {
	return m.reader.Read(output)
}

func (m *memoryReadWriteCloser) Write(data []byte) (int, error) {
	return m.writes.Write(data)
}

func (*memoryReadWriteCloser) Close() error { return nil }

func dbusTestFrame(order binary.ByteOrder, body []byte) []byte {
	header := make([]byte, 16)
	if order == binary.LittleEndian {
		header[0] = 'l'
	} else {
		header[0] = 'B'
	}
	header[1] = 2
	header[3] = 1
	order.PutUint32(header[4:8], uint32(len(body)))
	order.PutUint32(header[8:12], 1)
	return append(header, body...)
}

func TestBoundedDBusConnPassesAuthenticationThenBoundsFrames(t *testing.T) {
	first := dbusTestFrame(binary.LittleEndian, []byte("first"))
	second := dbusTestFrame(binary.BigEndian, []byte("second"))
	transport := &memoryReadWriteCloser{reader: bytes.NewReader(append([]byte("OK abcdef\r\n"), append(first, second...)...))}
	connection := &boundedDBusConn{ReadWriteCloser: transport}

	auth := make([]byte, len("OK abcdef\r\n"))
	_, err := io.ReadFull(connection, auth)
	require.NoError(t, err)
	assert.Equal(t, "OK abcdef\r\n", string(auth))
	_, err = connection.Write(dbusAuthBegin)
	require.NoError(t, err)

	frames, err := io.ReadAll(connection)
	require.NoError(t, err)
	assert.Equal(t, append(first, second...), frames)
	assert.Equal(t, dbusAuthBegin, transport.writes.Bytes())
}

func TestBoundedDBusConnAcceptsCoalescedAuthenticationBeginWrite(t *testing.T) {
	frame := dbusTestFrame(binary.LittleEndian, []byte("reply"))
	tests := []struct {
		name  string
		write []byte
	}{
		{name: "preceding bytes", write: append([]byte("AUTH EXTERNAL 30\r\n"), dbusAuthBegin...)},
		{name: "following bytes", write: append(append([]byte(nil), dbusAuthBegin...), dbusTestFrame(binary.LittleEndian, []byte("hello"))...)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &memoryReadWriteCloser{reader: bytes.NewReader(frame)}
			connection := &boundedDBusConn{ReadWriteCloser: transport}

			n, err := connection.Write(test.write)
			require.NoError(t, err)
			assert.Equal(t, len(test.write), n)
			assert.True(t, connection.binaryMode.Load())
			assert.Equal(t, test.write, transport.writes.Bytes())

			data, err := io.ReadAll(connection)
			require.NoError(t, err)
			assert.Equal(t, frame, data)
		})
	}
}

func TestBoundedDBusConnRejectsOversizedFrameBeforeExposingHeader(t *testing.T) {
	header := dbusTestFrame(binary.LittleEndian, nil)[:16]
	binary.LittleEndian.PutUint32(header[4:8], maxManagerDBusMessageSize)
	transport := &memoryReadWriteCloser{reader: bytes.NewReader(header)}
	connection := &boundedDBusConn{ReadWriteCloser: transport}
	_, err := connection.Write(dbusAuthBegin)
	require.NoError(t, err)

	output := make([]byte, 16)
	n, err := connection.Read(output)
	require.Error(t, err)
	assert.Zero(t, n)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestBoundedDBusConnRejectsInvalidEndianBeforeDecode(t *testing.T) {
	header := make([]byte, 16)
	header[0] = 'x'
	transport := &memoryReadWriteCloser{reader: bytes.NewReader(header)}
	connection := &boundedDBusConn{ReadWriteCloser: transport}
	_, err := connection.Write(dbusAuthBegin)
	require.NoError(t, err)

	_, err = connection.Read(make([]byte, 1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "byte order")
}

func TestBoundedDBusConnBoundsAuthenticationInput(t *testing.T) {
	transport := &memoryReadWriteCloser{reader: bytes.NewReader(bytes.Repeat([]byte{'x'}, maxManagerDBusAuthBytes+1))}
	connection := &boundedDBusConn{ReadWriteCloser: transport}

	data, err := io.ReadAll(connection)
	require.Error(t, err)
	assert.Len(t, data, maxManagerDBusAuthBytes)
	assert.Contains(t, err.Error(), "authentication response exceeds")
}

func TestBoundedDBusMessageSizeIncludesHeaderPadding(t *testing.T) {
	header := dbusTestFrame(binary.LittleEndian, nil)[:16]
	binary.LittleEndian.PutUint32(header[4:8], 7)
	binary.LittleEndian.PutUint32(header[12:16], 9)
	total, err := boundedDBusMessageSize(header)
	require.NoError(t, err)
	assert.Equal(t, uint64(16+16+7), total)
}
