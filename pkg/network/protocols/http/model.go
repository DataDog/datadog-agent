// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// Transaction is the interface for a HTTP transaction.
type Transaction interface {
	RequestLatency() float64
	ConnTuple() types.ConnectionKey
	Method() Method
	SetRequestMethod(Method)
	StatusCode() uint16
	SetStatusCode(uint16)
	StaticTags() uint64
	DynamicTags() []string
	String() string
	Incomplete() bool
	Path(buffer []byte) ([]byte, bool)
	ResponseLastSeen() uint64
	SetResponseLastSeen(ls uint64)
	RequestStarted() uint64
}

func computePath(targetBuffer, requestBuffer []byte) ([]byte, bool) {
	bLen := bytes.IndexByte(requestBuffer, 0)
	if bLen == -1 {
		bLen = len(requestBuffer)
	}
	// trim null byte + after
	b := requestBuffer[:bLen]
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
	for j = 0; j < len(b) && b[j] != ' ' && b[j] != '?'; j++ { //nolint:revive
		// revive complains about the empty block.  However, the empty block is what
		// we want.  We're just searching for the index of the last place of interest
		// of the string.  the resulting `j` is what's interesting
	}
	n := copy(targetBuffer, b[:j])
	// indicate if we knowingly captured the entire path
	fullPath := n < len(b)
	return targetBuffer[:n], fullPath
}
