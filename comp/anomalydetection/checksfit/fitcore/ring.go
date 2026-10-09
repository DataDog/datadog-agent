// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"encoding/binary"
	"math"
	"sync/atomic"
)

// Ring geometry: every record is 8 header bytes plus payload, aligned up to
// an eight-byte boundary; a record never crosses the physical end; eight
// bytes stay permanently unused to distinguish full from empty.
const (
	recordHeader = 8
	gap          = 8
)

// Record is one unencoded queue record. The payload is copied into the ring;
// no Go pointers, slice headers, or process-local layouts are shared.
type Record struct {
	Kind    uint32
	Payload []byte
}

// Rejection describes why a batch stopped accepting records.
type Rejection uint8

const (
	// RejectionNone reports that the queue accepted every offered record.
	RejectionNone Rejection = iota
	// RejectionFull reports that unread records filled the queue; the
	// producer drops incoming records without blocking.
	RejectionFull
	// RejectionInvalidType reports a record type outside the descriptor.
	RejectionInvalidType
	// RejectionOversized reports a record larger than the derived maximum.
	RejectionOversized
)

func (r Rejection) String() string {
	switch r {
	case RejectionFull:
		return "full"
	case RejectionInvalidType:
		return "invalid type"
	case RejectionOversized:
		return "oversized"
	default:
		return "none"
	}
}

// SendResult reports how many records were published, the first rejection
// that stopped the batch, and an independent notification failure. Once
// accepted, records are published even if notification fails; do not retry
// them automatically.
type SendResult struct {
	Accepted          int
	Rejection         Rejection
	NotificationError error
}

// sendBatch publishes the fitting prefix of records in one batch and calls
// wake once if anything was published. The wake function is injectable for
// tests.
func (m *mapping) sendBatch(records []Record, protocol ProtocolDescriptor, wake func(*uint32) error) (SendResult, error) {
	_, cursor, err := m.indexes()
	if err != nil {
		return SendResult{}, err
	}
	accepted := 0
	rejection := RejectionNone
	published := false
	ring := m.ring()
	for _, record := range records {
		if !protocol.supports(record.Kind) {
			rejection = RejectionInvalidType
			break
		}
		payloadLen := len(record.Payload)
		if uint64(payloadLen)+recordHeader > math.MaxUint32 {
			rejection = RejectionOversized
			break
		}
		rawSize := recordHeader + payloadLen
		size := (rawSize + 7) &^ 7
		if size > m.capacity-gap {
			rejection = RejectionOversized
			break
		}
		read := int(atomic.LoadUint32(m.word(readOffset)))
		if read >= m.capacity || read%8 != 0 {
			return SendResult{}, invalid("invalid read index")
		}
		used := cursor - read
		if cursor < read {
			used = m.capacity - read + cursor
		}
		if used > m.capacity-gap {
			return SendResult{}, invalid("queue exceeds reserved gap")
		}
		free := m.capacity - gap - used
		tail := m.capacity - cursor
		if size > tail {
			if tail > free {
				rejection = RejectionFull
				break
			}
			// Publish a zero wrap marker at the physical tail; the
			// consumer skips the entire tail span without a message.
			clear(ring[cursor : cursor+recordHeader])
			cursor = 0
			published = true
			if tail+size > free {
				rejection = RejectionFull
				break
			}
		} else if size > free {
			rejection = RejectionFull
			break
		}
		at := ring[cursor : cursor+size]
		binary.LittleEndian.PutUint32(at[0:4], uint32(payloadLen))
		binary.LittleEndian.PutUint32(at[4:8], record.Kind)
		copy(at[recordHeader:recordHeader+payloadLen], record.Payload)
		clear(at[rawSize:size])
		cursor = (cursor + size) % m.capacity
		accepted++
		published = true
	}
	var notification error
	if published {
		word := m.word(writeOffset)
		atomic.StoreUint32(word, uint32(cursor))
		if err := wake(word); err != nil {
			notification = err
		}
	}
	return SendResult{Accepted: accepted, Rejection: rejection, NotificationError: notification}, nil
}

// receiveInner returns one record, or cancelled=true when the wait registry
// was cancelled before a record became available. The payload is copied into
// an owned slice before the read index is released, so returned bytes stay
// valid after the ring reuses them.
func (m *mapping) receiveInner(protocol ProtocolDescriptor, reg *waitRegistry) (kind uint32, payload []byte, cancelled bool, err error) {
	ring := m.ring()
	for {
		if reg != nil && reg.check() {
			return 0, nil, true, nil
		}
		read, write, err := m.indexes()
		if err != nil {
			return 0, nil, false, err
		}
		if read == write {
			word := m.word(writeOffset)
			if reg != nil {
				// Always wait with the snapshot that established
				// emptiness; reloading a newer value can miss wakes.
				proceed, waitErr := reg.waitOn(word, uint32(write))
				if waitErr != nil {
					return 0, nil, false, waitErr
				}
				if !proceed {
					return 0, nil, true, nil
				}
			} else {
				if err := waitWord(word, uint32(write)); err != nil {
					return 0, nil, false, err
				}
			}
			continue
		}
		span := write - read
		if write <= read {
			span = m.capacity - read
		}
		if span < recordHeader {
			return 0, nil, false, invalid("published span lacks record header")
		}
		header := ring[read : read+recordHeader]
		length := binary.LittleEndian.Uint32(header[0:4])
		recordKind := binary.LittleEndian.Uint32(header[4:8])
		if recordKind == 0 {
			// A wrap marker must be zero-length, away from the ring
			// start, and fully covered by the acquired publication.
			if length != 0 || read == 0 || write >= read {
				return 0, nil, false, invalid("invalid wrap marker")
			}
			atomic.StoreUint32(m.word(readOffset), 0)
			continue
		}
		if !protocol.supports(recordKind) {
			return 0, nil, false, invalid("unknown record type")
		}
		size := (uint64(length) + recordHeader + 7) &^ 7
		if size > uint64(m.capacity-gap) || size > uint64(span) {
			return 0, nil, false, invalid("record exceeds published contiguous span")
		}
		data := make([]byte, length)
		copy(data, ring[read+recordHeader:read+recordHeader+int(length)])
		atomic.StoreUint32(m.word(readOffset), uint32((read+int(size))%m.capacity))
		return recordKind, data, false, nil
	}
}

// indexes acquire-loads both shared indexes and validates their range and
// alignment before any address calculation.
func (m *mapping) indexes() (read int, write int, err error) {
	r := int(atomic.LoadUint32(m.word(readOffset)))
	w := int(atomic.LoadUint32(m.word(writeOffset)))
	if r >= m.capacity || w >= m.capacity || r%8 != 0 || w%8 != 0 {
		return 0, 0, invalid("queue index is outside the aligned ring")
	}
	return r, w, nil
}
