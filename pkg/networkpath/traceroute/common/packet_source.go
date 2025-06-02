// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PacketSource is an interface representing ethernet packet capture
type PacketSource interface {
	// SetReadDeadline sets the deadline for when a Read() call must finish
	SetReadDeadline(t time.Time) error
	// Read reads a packet (including the ethernet frame)
	Read(buf []byte) (int, error)
	// Close closes the socket
	Close() error
}

// ReadAndParse reads from the given source into the buffer, and parses it with parser
func ReadAndParse(source PacketSource, buffer []byte, parser *FrameParser) error {
	n, err := source.Read(buffer)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return &ReceiveProbeNoPktError{Err: err}
	}
	if err != nil {
		return fmt.Errorf("ConnHandle failed to Read: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("ConnHandle Read() returned 0 bytes")
	}

	err = parser.Parse(buffer[:n])
	if err != nil {
		log.DebugFunc(func() string {
			data := hex.EncodeToString(buffer[:n])
			return fmt.Sprintf("error parsing packet of length %d: %s, %s", n, err, data)
		})
		return &BadPacketError{Err: fmt.Errorf("sackDriver failed to parse packet of length %d: %w", n, err)}
	}

	return nil
}
