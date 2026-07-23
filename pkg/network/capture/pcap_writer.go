// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"encoding/binary"
	"io"
)

// PCAP file format constants.
// Reference: https://wiki.wireshark.org/Development/LibpcapFileFormat
const (
	pcapMagicNumber  uint32 = 0xa1b2c3d4 // native byte-order magic (microsecond timestamps)
	pcapVersionMajor uint16 = 2
	pcapVersionMinor uint16 = 4
	pcapLinkTypeEth  uint32 = 1 // LINKTYPE_ETHERNET
)

// writePCAPHeader writes the 24-byte PCAP global file header to w.
//
// Layout (all little-endian):
//
//	  0  magic_number   (4 bytes) = 0xa1b2c3d4
//	  4  version_major  (2 bytes) = 2
//	  6  version_minor  (2 bytes) = 4
//	  8  thiszone       (4 bytes) = 0  (UTC)
//	 12  sigfigs        (4 bytes) = 0
//	 16  snaplen        (4 bytes)
//	 20  link_type      (4 bytes) = 1 (Ethernet)
func writePCAPHeader(w io.Writer, snapLen uint32) error {
	hdr := [24]byte{}
	le := binary.LittleEndian
	le.PutUint32(hdr[0:], pcapMagicNumber)
	le.PutUint16(hdr[4:], pcapVersionMajor)
	le.PutUint16(hdr[6:], pcapVersionMinor)
	le.PutUint32(hdr[8:], 0)  // thiszone (GMT offset) = 0
	le.PutUint32(hdr[12:], 0) // sigfigs = 0
	le.PutUint32(hdr[16:], snapLen)
	le.PutUint32(hdr[20:], pcapLinkTypeEth)
	_, err := w.Write(hdr[:])
	return err
}

// writePCAPPacket writes a single PCAP packet record (16-byte header + data) to w.
//
// Per-packet header layout (all little-endian):
//
//	  0  ts_sec   (4 bytes) — seconds since epoch
//	  4  ts_usec  (4 bytes) — microseconds
//	  8  incl_len (4 bytes) — number of bytes in the following Data field
//	 12  orig_len (4 bytes) — original on-wire packet length
func writePCAPPacket(w io.Writer, pkt RawPacket) error {
	sec := uint32(pkt.Timestamp.Unix())
	usec := uint32(pkt.Timestamp.Nanosecond() / 1000)
	inclLen := uint32(len(pkt.Data))
	origLen := pkt.OrigLen
	if origLen == 0 {
		origLen = inclLen
	}

	hdr := [16]byte{}
	le := binary.LittleEndian
	le.PutUint32(hdr[0:], sec)
	le.PutUint32(hdr[4:], usec)
	le.PutUint32(hdr[8:], inclLen)
	le.PutUint32(hdr[12:], origLen)

	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(pkt.Data)
	return err
}
