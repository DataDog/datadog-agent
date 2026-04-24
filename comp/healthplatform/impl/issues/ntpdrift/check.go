// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ntpdrift

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

const (
	// driftThreshold is the maximum acceptable clock drift before raising an issue
	driftThreshold = time.Minute

	// ntpServer is the NTP server used for clock synchronisation checks
	ntpServer = "pool.ntp.org:123"

	// ntpTimeout is the deadline for the NTP UDP exchange
	ntpTimeout = 5 * time.Second

	// ntpPacketSize is the size of an NTPv3 packet in bytes
	ntpPacketSize = 48

	// ntpEpochOffset is the number of seconds between 1 Jan 1900 (NTP epoch) and 1 Jan 1970 (Unix epoch)
	ntpEpochOffset = 2208988800
)

// Check queries pool.ntp.org and reports an issue when the local clock differs
// by more than driftThreshold from the NTP reference time.
func Check() (*healthplatform.IssueReport, error) {
	offset, err := queryNTPOffset(ntpServer)
	if err != nil {
		// If the NTP server is unreachable we skip the check rather than
		// raising a false-positive — network issues are a separate concern.
		return nil, nil //nolint:nilerr
	}

	if abs(offset) <= driftThreshold {
		return nil, nil
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"drift":     formatDuration(offset),
			"ntpServer": ntpServer,
		},
		Tags: []string{"ntp", "clock-drift", "timestamps"},
	}, nil
}

// queryNTPOffset performs a minimal NTPv3 client request and returns the
// difference between the NTP transmit timestamp and local time.
// A positive offset means the local clock is behind NTP time.
func queryNTPOffset(server string) (time.Duration, error) {
	conn, err := net.DialTimeout("udp", server, ntpTimeout)
	if err != nil {
		return 0, fmt.Errorf("ntp dial: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(ntpTimeout)); err != nil {
		return 0, fmt.Errorf("ntp set deadline: %w", err)
	}

	// Build a minimal NTPv3 client request packet.
	// Byte 0: LI=0, VN=3, Mode=3 → 0b00_011_011 = 0x1B
	req := make([]byte, ntpPacketSize)
	req[0] = 0x1B

	t1 := time.Now()
	if _, err := conn.Write(req); err != nil {
		return 0, fmt.Errorf("ntp write: %w", err)
	}

	resp := make([]byte, ntpPacketSize)
	if _, err := conn.Read(resp); err != nil {
		return 0, fmt.Errorf("ntp read: %w", err)
	}
	t4 := time.Now()

	// Transmit Timestamp (T3) is at bytes 40–47 (seconds in the upper 32 bits).
	ntpSecs := binary.BigEndian.Uint32(resp[40:44])
	ntpFrac := binary.BigEndian.Uint32(resp[44:48])

	// Convert NTP timestamp to Unix time.
	unixSecs := int64(ntpSecs) - ntpEpochOffset
	unixNanos := int64(ntpFrac) * 1e9 >> 32
	ntpTime := time.Unix(unixSecs, unixNanos)

	// Round-trip midpoint approximation for the local time at T3.
	localMid := t1.Add(t4.Sub(t1) / 2)

	return ntpTime.Sub(localMid), nil
}

// abs returns the absolute value of d.
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// formatDuration formats a duration as a signed human-readable string,
// e.g. "+2m30s" or "-45s".
func formatDuration(d time.Duration) string {
	if d >= 0 {
		return "+" + d.Round(time.Second).String()
	}
	return d.Round(time.Second).String()
}
