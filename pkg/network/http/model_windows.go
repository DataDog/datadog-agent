// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/etw"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"golang.org/x/sys/windows"
)

const HTTPBufferSize = driver.HttpBufferSize
const HTTPBatchSize = driver.HttpBatchSize

var (
	verb2method = []Method{
		// from HTTP_VERB enumeration (http.h) (etw-http-service.go)
		//
		// looks like MS does not define verb for PATCH method
		//
		MethodUnknown, // HttpVerbUnparsed uint32 = iota
		MethodUnknown, // HttpVerbUnknown
		MethodUnknown, // HttpVerbInvalid
		MethodOptions, // HttpVerbOPTIONS
		MethodGet,     // HttpVerbGET
		MethodHead,    // HttpVerbHEAD
		MethodPost,    // HttpVerbPOST
		MethodPut,     // HttpVerbPUT
		MethodDelete,  // HttpVerbDELETE
		MethodUnknown, // HttpVerbTRACE
		MethodUnknown, // HttpVerbCONNECT
		MethodUnknown, // HttpVerbTRACK
		MethodUnknown, // HttpVerbMOVE
		MethodUnknown, // HttpVerbCOPY
		MethodUnknown, // HttpVerbPROPFIND
		MethodUnknown, // HttpVerbPROPPATCH
		MethodUnknown, // HttpVerbMKCOL
		MethodUnknown, // HttpVerbLOCK
		MethodUnknown, // HttpVerbUNLOCK
		MethodUnknown, // HttpVerbSEARCH
		MethodUnknown, // HttpVerbMaximum
	}
)

type httpTX interface {
	ReqFragment() []byte
	StatusClass() int
	RequestLatency() float64
	isIPV4() bool
	SrcIPLow() uint64
	SrcIPHigh() uint64
	SrcPort() uint16
	DstIPLow() uint64
	DstIPHigh() uint64
	DstPort() uint16
	Method() Method
	StatusCode() uint16
	Tags() uint64
	Incomplete() bool
}

type driverHttpTX driver.HttpTransactionType
type etwHttpTX etw.ConnHttp

// errLostBatch isn't a valid error in windows
var errLostBatch = errors.New("invalid error")

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *driverHttpTX) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func (tx *driverHttpTX) StatusClass() int {
	return (int(tx.ResponseStatusCode) / 100) * 100
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *driverHttpTX) RequestLatency() float64 {
	return nsTimestampToFloat(uint64(tx.ResponseLastSeen - tx.RequestStarted))
}

func (tx *driverHttpTX) isIPV4() bool {
	return tx.Tup.Family == windows.AF_INET
}

func (tx *driverHttpTX) SrcIPLow() uint64 {
	// Source & dest IP are given to us as a 16-byte slices in network byte order (BE). To convert to
	// low/high representation, we must convert to host byte order (LE).
	if tx.isIPV4() {
		return uint64(binary.LittleEndian.Uint32(tx.Tup.CliAddr[:4]))
	}
	return binary.LittleEndian.Uint64(tx.Tup.CliAddr[8:])
}

func (tx *driverHttpTX) SrcIPHigh() uint64 {
	if tx.isIPV4() {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(tx.Tup.CliAddr[:8])
}

func (tx *driverHttpTX) SrcPort() uint16 {
	return tx.Tup.CliPort
}

func (tx *driverHttpTX) DstIPLow() uint64 {
	if tx.isIPV4() {
		return uint64(binary.LittleEndian.Uint32(tx.Tup.SrvAddr[:4]))
	}
	return binary.LittleEndian.Uint64(tx.Tup.SrvAddr[8:])
}

func (tx *driverHttpTX) DstIPHigh() uint64 {
	if tx.isIPV4() {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(tx.Tup.SrvAddr[:8])
}

func (tx *driverHttpTX) DstPort() uint16 {
	return tx.Tup.SrvPort
}

func (tx *driverHttpTX) Method() Method {
	return Method(tx.RequestMethod)
}

func (tx *driverHttpTX) StatusCode() uint16 {
	return tx.ResponseStatusCode
}

// Tags are not part of windows http transactions
func (tx *driverHttpTX) Tags() uint64 {
	return 0
}

// Incomplete transactions does not apply to windows
func (tx *driverHttpTX) Incomplete() bool {
	return false
}

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *etwHttpTX) ReqFragment() []byte {
	return []byte(tx.Http.Url)
}

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func (tx *etwHttpTX) StatusClass() int {
	return (int(tx.Http.StatusCode) / 100) * 100
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *etwHttpTX) RequestLatency() float64 {
	return nsTimestampToFloat(uint64(tx.Http.RespTime.Sub(tx.Http.ReqTime)))
}

func (tx *etwHttpTX) isIPV4() bool {
	return tx.Conn.Local.Is4()
}

func (tx *etwHttpTX) SrcIPLow() uint64 {
	// Source & dest IP are given to us as a 16-byte slices in network byte order (BE). To convert to
	// low/high representation, we must convert to host byte order (LE).
	if tx.isIPV4() {
		return uint64(binary.LittleEndian.Uint32(tx.Conn.Remote.As4()))
	}
	return binary.LittleEndian.Uint64(tx.Conn.Remote.As16()[8:])
}

func (tx *etwHttpTX) SrcIPHigh() uint64 {
	if tx.isIPV4() {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(tx.Conn.Remote.As16()[:8])
}

func (tx *etwHttpTX) SrcPort() uint16 {
	return tx.Conn.RemotePort
}

func (tx *etwHttpTX) DstIPLow() uint64 {
	if tx.isIPV4() {
		return uint64(binary.LittleEndian.Uint32(tx.Conn.Local.As4()))
	}
	return binary.LittleEndian.Uint64(tx.Conn.Local.As16()[8:])
}

func (tx *etwHttpTX) DstIPHigh() uint64 {
	if tx.isIPV4() {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(tx.Conn.Local.As16()[:8])
}

func (tx *etwHttpTX) DstPort() uint16 {
	return tx.Conn.LocalPort
}

func (tx *etwHttpTX) Method() Method {
	if tx.Http.Verb >= etw.HttpVerbMaximum {
		return MethodUnknown
	}

	return verb2method[tx.Http.Verb]
}

func (tx *etwHttpTX) StatusCode() uint16 {
	return tx.Http.StatusCode
}

// Tags are not part of windows http transactions
func (tx *etwHttpTX) Tags() uint64 {
	return 0
}

// Incomplete transactions does not apply to windows
func (tx *etwHttpTX) Incomplete() bool {
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

// generateIPv4HTTPTransaction is a testing helper function required for the http_statkeeper tests
func generateIPv4HTTPTransaction(client util.Address, server util.Address, cliPort int, srvPort int, path string, code int, latency time.Duration) httpTX {
	var tx driverHttpTX

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := uint64(uint64(latency))
	cli := client.Bytes()
	srv := server.Bytes()

	tx.RequestStarted = 1
	tx.ResponseLastSeen = tx.RequestStarted + latencyNS
	tx.ResponseStatusCode = uint16(code)
	for i := 0; i < len(tx.RequestFragment) && i < len(reqFragment); i++ {
		tx.RequestFragment[i] = uint8(reqFragment[i])
	}
	for i := 0; i < len(tx.Tup.CliAddr) && i < len(cli); i++ {
		tx.Tup.CliAddr[i] = cli[i]
	}
	for i := 0; i < len(tx.Tup.SrvAddr) && i < len(srv); i++ {
		tx.Tup.SrvAddr[i] = srv[i]
	}
	tx.Tup.CliPort = uint16(cliPort)
	tx.Tup.SrvPort = uint16(srvPort)

	return (httpTX)tx
}
