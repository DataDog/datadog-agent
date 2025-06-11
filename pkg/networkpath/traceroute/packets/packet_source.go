// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package packets has packet capture/emitting/filtering logic
package packets

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Source is an interface representing ethernet packet capture
type Source interface {
	// SetReadDeadline sets the deadline for when a Read() call must finish
	SetReadDeadline(t time.Time) error
	// Read reads a packet (starting with the IP frame)
	Read(buf []byte) (int, error)
	// Close closes the socket
	Close() error
}

// ReadAndParse reads from the given source into the buffer, and parses it with parser
func ReadAndParse(source Source, buffer []byte, parser *FrameParser) error {
	n, err := source.Read(buffer)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return &common.ReceiveProbeNoPktError{Err: err}
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
		return &common.BadPacketError{Err: fmt.Errorf("sackDriver failed to parse packet of length %d: %w", n, err)}
	}

	return nil
}
