// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"
)

const (
	maxManagerDBusMessageSize = 1024 * 1024
	maxManagerDBusAuthBytes   = 4 * 1024
)

var dbusAuthBegin = []byte("BEGIN\r\n")

// boundedDBusConn leaves the line-oriented authentication exchange untouched,
// then validates the fixed header of every incoming D-Bus message before the
// decoder can observe it. godbus otherwise accepts messages up to 128 MiB and
// allocates the complete body before application-level field validation.
type boundedDBusConn struct {
	io.ReadWriteCloser

	binaryMode atomic.Bool
	authBytes  int
	authLine   int
	header     [16]byte
	headerOff  int
	remaining  uint64
}

func (c *boundedDBusConn) Write(data []byte) (int, error) {
	if bytes.Contains(data, dbusAuthBegin) {
		// BEGIN\r\n is a locally generated protocol marker, so it is safe to detect
		// inside a coalesced write rather than depending on godbus preserving an
		// isolated Write call.
		// Publish the mode first so a fast bus reply cannot race the transition.
		c.binaryMode.Store(true)
	}
	return c.ReadWriteCloser.Write(data)
}

func (c *boundedDBusConn) Read(output []byte) (int, error) {
	if len(output) == 0 {
		return c.ReadWriteCloser.Read(output)
	}
	if !c.binaryMode.Load() {
		remainingTotal := maxManagerDBusAuthBytes - c.authBytes
		remainingLine := maxManagerDBusAuthBytes - c.authLine
		if remainingTotal <= 0 || remainingLine <= 0 {
			return 0, fmt.Errorf("systemd manager D-Bus authentication response exceeds %d bytes", maxManagerDBusAuthBytes)
		}
		limit := len(output)
		if limit > remainingTotal {
			limit = remainingTotal
		}
		if limit > remainingLine {
			limit = remainingLine
		}
		n, err := c.ReadWriteCloser.Read(output[:limit])
		c.authBytes += n
		for _, character := range output[:n] {
			c.authLine++
			if character == '\n' {
				c.authLine = 0
			}
		}
		return n, err
	}
	if c.headerOff == 0 && c.remaining == 0 {
		if _, err := io.ReadFull(c.ReadWriteCloser, c.header[:]); err != nil {
			return 0, err
		}
		total, err := boundedDBusMessageSize(c.header[:])
		if err != nil {
			return 0, err
		}
		c.remaining = total - uint64(len(c.header))
	}
	if c.headerOff < len(c.header) {
		n := copy(output, c.header[c.headerOff:])
		c.headerOff += n
		if c.headerOff == len(c.header) && c.remaining == 0 {
			c.headerOff = 0
		}
		return n, nil
	}

	limit := len(output)
	if uint64(limit) > c.remaining {
		limit = int(c.remaining)
	}
	n, err := c.ReadWriteCloser.Read(output[:limit])
	if uint64(n) > c.remaining {
		return 0, fmt.Errorf("systemd manager D-Bus reader crossed a message boundary")
	}
	c.remaining -= uint64(n)
	if c.remaining == 0 {
		c.headerOff = 0
	}
	return n, err
}

func boundedDBusMessageSize(header []byte) (uint64, error) {
	if len(header) != 16 {
		return 0, fmt.Errorf("systemd manager D-Bus fixed header has %d bytes; expected 16", len(header))
	}
	var order binary.ByteOrder
	switch header[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return 0, fmt.Errorf("systemd manager D-Bus message has invalid byte order")
	}
	bodySize := uint64(order.Uint32(header[4:8]))
	headerFieldsSize := uint64(order.Uint32(header[12:16]))
	paddedHeaderFieldsSize := (headerFieldsSize + 7) &^ uint64(7)
	total := uint64(len(header)) + paddedHeaderFieldsSize + bodySize
	if total > maxManagerDBusMessageSize {
		return 0, fmt.Errorf("systemd manager D-Bus message exceeds %d bytes", maxManagerDBusMessageSize)
	}
	return total, nil
}
