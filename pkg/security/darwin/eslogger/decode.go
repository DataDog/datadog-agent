// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eslogger

import (
	"bufio"
	"encoding/json"
	"io"
)

// knownKinds are the event kinds the translator handles. Anything else is
// counted and skipped, so a macOS release that adds event types cannot break
// the collector.
var knownKinds = map[string]bool{
	"exec": true,
	"fork": true,
	"exit": true,
}

const (
	initialScanBuf = 64 * 1024
	// A single exec message carries argv, the environment and a stat block per
	// file, so lines run to tens of kilobytes. Allow a generous maximum.
	maxScanBuf = 8 * 1024 * 1024
)

// Stats counts what the decoder saw.
type Stats struct {
	Lines     uint64
	Decoded   uint64
	Unknown   uint64
	Malformed uint64
	// Dropped is inferred from gaps in global_seq_num. Endpoint Security drops
	// messages under load without reporting it, so this is the only fidelity
	// signal available.
	Dropped uint64
}

// Decoder reads eslogger JSON-Lines output.
type Decoder struct {
	scanner *bufio.Scanner
	stats   Stats

	lastSeq     uint64
	haveLastSeq bool
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, initialScanBuf), maxScanBuf)
	return &Decoder{scanner: s}
}

// Next returns the next message of a known kind, or io.EOF when the stream
// ends. Malformed lines and unknown kinds are counted and skipped.
func (d *Decoder) Next() (*Message, error) {
	for d.scanner.Scan() {
		line := d.scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		d.stats.Lines++

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			d.stats.Malformed++
			continue
		}

		// Account for sequence numbers before filtering on kind, so that
		// skipping an unknown kind is not mistaken for a dropped message.
		d.noteSeq(msg.GlobalSeqNum)

		kind, err := msg.Kind()
		if err != nil {
			d.stats.Malformed++
			continue
		}
		if !knownKinds[kind] {
			d.stats.Unknown++
			continue
		}

		d.stats.Decoded++
		return &msg, nil
	}

	if err := d.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// noteSeq accumulates dropped-message counts from gaps in global_seq_num.
func (d *Decoder) noteSeq(seq uint64) {
	if d.haveLastSeq && seq > d.lastSeq+1 {
		d.stats.Dropped += seq - d.lastSeq - 1
	}
	d.lastSeq = seq
	d.haveLastSeq = true
}

// Stats returns the decoder counters.
func (d *Decoder) Stats() Stats { return d.stats }
