// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procnet

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"os"
	"slices"
	"strconv"
)

// This file contains the code that does the parsing of /proc/net/{tcp,tcp6} files.
//
// The format of these files is the following:
//
//    46: 010310AC:9C4C 030310AC:1770 01
//    |      |      |      |      |   |--> connection state
//    |      |      |      |      |------> remote TCP port number
//    |      |      |      |-------------> remote IPv4 address
//    |      |      |--------------------> local TCP port number
//    |      |---------------------------> local IPv4 address
//    |----------------------------------> number of entry
//
//    00000150:00000000 01:00000019 00000000
//       |        |     |     |       |--> number of unrecovered RTO timeouts
//       |        |     |     |----------> number of jiffies until timer expires
//       |        |     |----------------> timer_active (see below)
//       |        |----------------------> receive-queue
//       |-------------------------------> transmit-queue
//
//    1000        0 54165785 4 cd1e6040 25 4 27 3 -1
//     |          |    |     |    |     |  | |  | |--> slow start size threshold,
//     |          |    |     |    |     |  | |  |      or -1 if the threshold
//     |          |    |     |    |     |  | |  |      is >= 0xFFFF
//     |          |    |     |    |     |  | |  |----> sending congestion window
//     |          |    |     |    |     |  | |-------> (ack.quick<<1)|ack.pingpong
//     |          |    |     |    |     |  |---------> Predicted tick of soft clock
//     |          |    |     |    |     |              (delayed ACK control data)
//     |          |    |     |    |     |------------> retransmit timeout
//     |          |    |     |    |------------------> location of socket in memory
//     |          |    |     |-----------------------> socket reference count
//     |          |    |-----------------------------> inode
//     |          |----------------------------------> unanswered 0-window probes
//     |---------------------------------------------> uid
//
// Source: https://www.kernel.org/doc/Documentation/networking/proc_net_tcp.txt

type scanner struct {
	file   *os.File
	reader *bufio.Reader

	hexDecodingBuffer []byte
}

func newScanner(filepath string) (*scanner, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(f)

	// Skip header line
	_, _ = reader.ReadBytes('\n')

	return &scanner{
		file:              f,
		reader:            reader,
		hexDecodingBuffer: make([]byte, 16),
	}, nil

}

func (s *scanner) Next() (entry, bool) {
	b, err := s.reader.ReadBytes('\n')
	if err != nil {
		return entry{}, false
	}

	return newEntry(b, s.hexDecodingBuffer), true
}

func (s *scanner) Close() {
	s.file.Close()
}

// entry represents one line in the /proc/net file
type entry struct {
	laddr []byte
	raddr []byte
	state []byte
	inode []byte

	// used for the purposes of decoding hex numbers
	buffer []byte
}

func newEntry(line, buffer []byte) entry {
	e := entry{buffer: buffer}
	iter := fieldIterator{line}

	// refer to the diagram above for the line format
	iter.skip(1) // skips the number of the entry
	e.laddr = iter.nextField()
	e.raddr = iter.nextField()
	e.state = iter.nextField()
	iter.skip(5) // skips queue (tx + rx), tr, tm->when, retrnsmt ....
	e.inode = iter.nextField()

	return e
}

func (e entry) LocalAddress() (netip.Addr, uint16) {
	return e.parseAddress(e.laddr)
}

func (e entry) RemoteAddress() (netip.Addr, uint16) {
	return e.parseAddress(e.raddr)
}

func (e entry) Inode() int {
	return e.atoi(e.inode)
}

func (e entry) ConnectionState() int {
	n, err := hex.Decode(e.buffer, e.state)
	if err != nil || n != 1 {
		return -1
	}

	return int(e.buffer[0])
}

func (e entry) atoi(number []byte) int {
	// note: looks like the Go compiler is smart enough to not allocate the
	// string from the given byte slice. There is a benchmark in parser_net.go
	// covering this.
	i, err := strconv.Atoi(string(number))
	if err != nil {
		return -1
	}

	return i
}

// parseAddress converts a textual address representation of the form
// 010310AC:9C4C into a (netip.Addr, uint16) pair.
func (e entry) parseAddress(rawAddress []byte) (netip.Addr, uint16) {
	i := bytes.IndexByte(rawAddress, ':')
	if i == -1 {
		return netip.Addr{}, 0
	}

	rawIP := rawAddress[:i]
	rawPort := rawAddress[i+1:]

	// Parse port number
	n, err := hex.Decode(e.buffer, rawPort)
	if err != nil {
		return netip.Addr{}, 0
	}
	parsedPort := binary.BigEndian.Uint16(e.buffer[:n])

	// Parse IP address (IPv4 or IPv6)
	n, err = hex.Decode(e.buffer, rawIP)
	if err != nil || (n != 4 && n != 16) {
		return netip.Addr{}, 0
	}
	// Byte ordering is reversed (big endian) than `netip` expects
	slices.Reverse(e.buffer[:n])

	if n == 4 {
		var ipv4 [4]byte
		copy(ipv4[:], e.buffer[:n])
		return netip.AddrFrom4(ipv4), parsedPort
	}

	var ipv6 [16]byte
	copy(ipv6[:], e.buffer[:n])
	return netip.AddrFrom16(ipv6), parsedPort
}

// copied from pkg/network/proc_net.go
type fieldIterator struct {
	data []byte
}

func (iter *fieldIterator) nextField() []byte {
	// skip any leading whitespace
	for i, b := range iter.data {
		if b != ' ' {
			iter.data = iter.data[i:]
			break
		}
	}

	// read field up until the first whitespace char
	var result []byte
	for i, b := range iter.data {
		if b == ' ' {
			result = iter.data[:i]
			iter.data = iter.data[i:]
			break
		}
	}

	return result
}

func (iter *fieldIterator) skip(n int) {
	for i := 0; i < n; i++ {
		_ = iter.nextField()
	}
}
