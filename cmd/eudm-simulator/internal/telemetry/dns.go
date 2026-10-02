// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"encoding/binary"
	"errors"
)

// ValidateConnectionDNSV2 checks the complete bounded V2 domain and lookup
// tables before using agent-payload's iterator, which discards some malformed
// lookup errors. It never copies domain strings or returns input in errors.
func ValidateConnectionDNSV2(database, lookups []byte) error {
	invalid := errors.New("invalid captured connection DNS encoding")
	const maxItems = 65536
	domains := dnsCursor{data: database}
	count, ok := domains.number()
	if !ok || count == 0 || count > maxItems {
		return invalid
	}
	// The historical domain-database middle offset is intentionally unused by
	// the Agent encoder and decoder. Only its varint framing is meaningful.
	if _, ok = domains.number(); !ok {
		return invalid
	}
	offsets := make(map[uint64]bool, int(count))
	for range count {
		offset := uint64(domains.at)
		length, ok := domains.number()
		if !ok || length == 0 || !domains.skip(length) {
			return invalid
		}
		offsets[offset] = true
	}
	if domains.at != len(database) || len(lookups) < 5 || lookups[0] != 2 {
		return invalid
	}
	bucketCount := int(binary.LittleEndian.Uint16(lookups[1:3]))
	if bucketCount == 0 {
		return invalid
	}
	header := dnsCursor{data: lookups, at: 3}
	positionLength, ok := header.number()
	if !ok {
		return invalid
	}
	middle, ok := header.number()
	if !ok || positionLength > uint64(len(lookups)-header.at) {
		return invalid
	}
	positions := dnsCursor{data: lookups[header.at : header.at+int(positionLength)]}
	buckets := dnsCursor{data: lookups[header.at+int(positionLength):]}
	keys := make(map[string]bool)
	associations := uint64(0)
	for bucket := range bucketCount {
		if bucket == bucketCount/2 && middle != uint64(positions.at) {
			return invalid
		}
		position, ok := positions.number()
		if !ok || position != uint64(buckets.at) {
			return invalid
		}
		entries, ok := buckets.number()
		if !ok || entries > uint64(maxItems-len(keys)) {
			return invalid
		}
		for range entries {
			length, ok := buckets.number()
			if !ok || length == 0 || length > 64 || length > uint64(len(buckets.data)-buckets.at) {
				return invalid
			}
			key := string(buckets.data[buckets.at : buckets.at+int(length)])
			buckets.at += int(length)
			if keys[key] {
				return invalid
			}
			keys[key] = true
			names, ok := buckets.number()
			if !ok || names > maxItems-associations {
				return invalid
			}
			associations += names
			for range names {
				offset, ok := buckets.number()
				if !ok || !offsets[offset] {
					return invalid
				}
			}
		}
	}
	if positions.at != len(positions.data) || buckets.at != len(buckets.data) {
		return invalid
	}
	return nil
}

type dnsCursor struct {
	data []byte
	at   int
}

func (c *dnsCursor) number() (uint64, bool) {
	value, consumed := binary.Uvarint(c.data[c.at:])
	if consumed <= 0 {
		return 0, false
	}
	c.at += consumed
	return value, true
}

func (c *dnsCursor) skip(size uint64) bool {
	if size > uint64(len(c.data)-c.at) {
		return false
	}
	c.at += int(size)
	return true
}
