// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"golang.org/x/sys/windows"
)

const HTTPBufferSize = driver.HttpBufferSize
const HTTPBatchSize = driver.HttpBatchSize

type httpTX FullHttpTransaction

// errLostBatch isn't a valid error in windows
var errLostBatch = errors.New("invalid error")

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *httpTX) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func (tx *httpTX) StatusClass() int {
	return (int(tx.Txn.ResponseStatusCode) / 100) * 100
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *httpTX) RequestLatency() float64 {
	return nsTimestampToFloat(uint64(tx.Txn.ResponseLastSeen - tx.Txn.RequestStarted))
}

func (tx *httpTX) isIPV4() bool {
	return tx.Txn.Tup.Family == windows.AF_INET
}

func (tx *httpTX) SrcIPLow() uint64 {
	// Source & dest IP are given to us as a 16-byte slices in network byte order (BE). To convert to
	// low/high representation, we must convert to host byte order (LE).
	if tx.isIPV4() {
		return uint64(binary.LittleEndian.Uint32(tx.Txn.Tup.CliAddr[:4]))
	}
	return binary.LittleEndian.Uint64(tx.Txn.Tup.CliAddr[8:])
}

func (tx *httpTX) SrcIPHigh() uint64 {
	if tx.isIPV4() {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(tx.Txn.Tup.CliAddr[:8])
}

func (tx *httpTX) SrcPort() uint16 {
	return tx.Txn.Tup.CliPort
}

func (tx *httpTX) DstIPLow() uint64 {
	if tx.isIPV4() {
		return uint64(binary.LittleEndian.Uint32(tx.Txn.Tup.SrvAddr[:4]))
	}
	return binary.LittleEndian.Uint64(tx.Txn.Tup.SrvAddr[8:])
}

func (tx *httpTX) DstIPHigh() uint64 {
	if tx.isIPV4() {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(tx.Txn.Tup.SrvAddr[:8])
}

func (tx *httpTX) DstPort() uint16 {
	return tx.Txn.Tup.SrvPort
}

func (tx *httpTX) Method() Method {
	return Method(tx.Txn.RequestMethod)
}

func (tx *httpTX) StatusCode() uint16 {
	return tx.Txn.ResponseStatusCode
}

// Tags are not part of windows http transactions
func (tx *httpTX) Tags() uint64 {
	return 0
}

func (tx *httpTX) String() string {
	var output strings.Builder
	output.WriteString("httpTX{")
	output.WriteString("Method: '" + tx.Method().String() + "', ")
	output.WriteString("Fragment: '" + hex.EncodeToString(tx.RequestFragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

// Windows does not have incomplete http transactions because flows in the windows driver
// see both directions of traffic
func (tx *httpTX) Incomplete() bool {
	return false
}

// below is copied from pkg/trace/stats/statsraw.go
// 10 bits precision (any value will be +/- 1/1024)
const roundMask uint64 = 1 << 10

// nsTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func nsTimestampToFloat(ns uint64) float64 {
	var shift uint
	for ns > roundMask {
		ns = ns >> 1
		shift++
	}
	return float64(ns << shift)
}
