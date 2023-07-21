// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PacketNow return the timestamp in nanoseconds relative to the kernel boot time
func PacketNow() int64 {
	now, err := ebpf.NowNanoseconds()
	if err != nil {
		log.Errorf("statkeeper ebpf.NowNanoseconds() failed %s", err)
		return 0
	}
	return now
}
func (tx *EbpfTx) Protocol() protocols.ProtocolType {
	return protocols.HTTP2
}

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (tx *EbpfTx) Path(buffer []byte) ([]byte, bool) {
	bLen := bytes.IndexByte(tx.Request_fragment[:], 0)
	if bLen == -1 {
		bLen = len(tx.Request_fragment)
	}
	// trim null byte + after
	b := tx.Request_fragment[:bLen]
	// find first space after request method
	i := bytes.IndexByte(b, ' ')
	i++
	// ensure we found a space, it isn't at the end, and the next chars are '/' or '*'
	if i == 0 || i == len(b) || (b[i] != '/' && b[i] != '*') {
		return nil, false
	}
	// trim to start of path
	b = b[i:]
	// capture until we find the slice end, a space, or a question mark (we ignore the query parameters)
	var j int
	for j = 0; j < len(b) && b[j] != ' ' && b[j] != '?'; j++ {
	}
	n := copy(buffer, b[:j])
	// indicate if we knowingly captured the entire path
	fullPath := n < len(b)
	return buffer[:n], fullPath
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *EbpfTx) RequestLatency() int64 {
	if uint64(tx.Request_started) == 0 || uint64(tx.Response_last_seen) == 0 {
		return 0
	}
	return int64(tx.Response_last_seen) - int64(tx.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *EbpfTx) Incomplete() bool {
	return tx.Request_started == 0 || tx.Response_status_code == 0
}

func (tx *EbpfTx) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: tx.Tup.Saddr_h,
		SrcIPLow:  tx.Tup.Saddr_l,
		DstIPHigh: tx.Tup.Daddr_h,
		DstIPLow:  tx.Tup.Daddr_l,
		SrcPort:   tx.Tup.Sport,
		DstPort:   tx.Tup.Dport,
	}
}

func (tx *EbpfTx) Method() Method {
	return Method(tx.Request_method)
}

func (tx *EbpfTx) StatusCode() uint16 {
	return tx.Response_status_code
}

func (tx *EbpfTx) SetStatusCode(code uint16) {
	tx.Response_status_code = code
}

func (tx *EbpfTx) ResponseLastSeen() uint64 {
	return tx.Response_last_seen
}

func (tx *EbpfTx) SetResponseLastSeen(lastSeen uint64) {
	tx.Response_last_seen = lastSeen

}
func (tx *EbpfTx) RequestStarted() uint64 {
	return tx.Request_started
}

func (tx *EbpfTx) SetRequestMethod(m Method) {
	tx.Request_method = uint8(m)
}

// StaticTags returns an uint64 representing the tags bitfields
// Tags are defined here : pkg/network/ebpf/kprobe_types.go
func (tx *EbpfTx) StaticTags() uint64 {
	return tx.Tags
}

func (tx *EbpfTx) DynamicTags() []string {
	return nil
}

func (tx *EbpfTx) String() string {
	var output strings.Builder
	output.WriteString("ebpfTx{")
	output.WriteString("Started: '" + strconv.FormatUint(tx.Request_started, 10) + "', ")
	output.WriteString("LastSeen: '" + strconv.FormatUint(tx.Response_last_seen, 10) + "', ")
	output.WriteString("ResponseStatusCode: '" + strconv.FormatUint(uint64(tx.Response_status_code), 10) + "', ")
	output.WriteString("Method: '" + Method(tx.Request_method).String() + "', ")
	output.WriteString("Tags: '0x" + strconv.FormatUint(tx.Tags, 16) + "', ")
	output.WriteString("Fragment: '" + hex.EncodeToString(tx.Request_fragment[:]) + "', ")
	output.WriteString("Tuple: '" + fmt.Sprintf("%#+v", tx.ConnTuple()) + "', ")
	output.WriteString("}")
	return output.String()
}

func requestFragment(fragment []byte) [BufferSize]byte {
	if len(fragment) >= BufferSize {
		return *(*[BufferSize]byte)(fragment)
	}
	var b [BufferSize]byte
	copy(b[:], fragment)
	return b
}
