// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"bytes"
	"encoding/binary"

	//"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/etw"
)

type etwHttpTX struct {
	//	httpTX
	*etw.Http
}

// errLostBatch isn't a valid error in windows
var errLostBatch = errors.New("invalid error")

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func statusClass(statusCode uint16) int {
	return (int(statusCode) / 100) * 100
}

func requestLatency(responseLastSeen uint64, requestStarted uint64) float64 {
	return nsTimestampToFloat(uint64(responseLastSeen - requestStarted))
}

func isIPV4(tup *driver.ConnTupleType) bool {
	return tup.Family == windows.AF_INET
}

func ipLow(isIp4 bool, addr [16]uint8) uint64 {
	// Source & dest IP are given to us as a 16-byte slices in network byte order (BE). To convert to
	// low/high representation, we must convert to host byte order (LE).
	if isIp4 {
		return uint64(binary.LittleEndian.Uint32(addr[:4]))
	}
	return binary.LittleEndian.Uint64(addr[8:])
}

func ipHigh(isIp4 bool, addr [16]uint8) uint64 {
	if isIp4 {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(addr[:8])
}

func srcIPLow(tup *driver.ConnTupleType) uint64 {
	return ipLow(isIPV4(tup), tup.CliAddr)
}

func srcIPHigh(tup *driver.ConnTupleType) uint64 {
	return ipHigh(isIPV4(tup), tup.CliAddr)
}

func dstIPLow(tup *driver.ConnTupleType) uint64 {
	return ipLow(isIPV4(tup), tup.SrvAddr)
}

func dstIPHigh(tup *driver.ConnTupleType) uint64 {
	return ipHigh(isIPV4(tup), tup.SrvAddr)
}

// --------------------------
//
// driverHttpTX interface
//

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *FullHttpTransaction) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

func (tx *FullHttpTransaction) StatusClass() int {
	return statusClass(tx.Txn.ResponseStatusCode)
}

func (tx *FullHttpTransaction) RequestLatency() float64 {
	return requestLatency(tx.Txn.ResponseLastSeen, tx.Txn.RequestStarted)
}

func (tx *FullHttpTransaction) isIPV4() bool {
	return isIPV4(&tx.Txn.Tup)
}

func (tx *FullHttpTransaction) SrcIPLow() uint64 {
	return srcIPLow(&tx.Txn.Tup)
}

func (tx *FullHttpTransaction) SrcIPHigh() uint64 {
	return srcIPHigh(&tx.Txn.Tup)
}

func (tx *FullHttpTransaction) SrcPort() uint16 {
	return tx.Txn.Tup.CliPort
}

func (tx *FullHttpTransaction) DstIPLow() uint64 {
	return dstIPLow(&tx.Txn.Tup)
}

func (tx *FullHttpTransaction) DstIPHigh() uint64 {
	return dstIPHigh(&tx.Txn.Tup)
}

func (tx *FullHttpTransaction) DstPort() uint16 {
	return tx.Txn.Tup.SrvPort
}

func (tx *FullHttpTransaction) Method() Method {
	return Method(tx.Txn.RequestMethod)
}

func (tx *FullHttpTransaction) StatusCode() uint16 {
	return tx.Txn.ResponseStatusCode
}

// Static Tags are not part of windows driver http transactions
func (tx *FullHttpTransaction) StaticTags() uint64 {
	return 0
}

// Dynamic Tags are not part of windows driver http transactions
func (tx *FullHttpTransaction) DynamicTags() []string {
	return nil
}

func (tx *FullHttpTransaction) String() string {
	var output strings.Builder
	output.WriteString("httpTX{")
	output.WriteString("\n  Method: '" + tx.Method().String() + "', ")
	output.WriteString("\n  MaxRequest: '" + strconv.Itoa(int(tx.Txn.MaxRequestFragment)) + "', ")
	//output.WriteString("Fragment: '" + hex.EncodeToString(tx.RequestFragment[:]) + "', ")
	output.WriteString("\n  Fragment: '" + string(tx.RequestFragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

// Windows does not have incomplete http transactions because flows in the windows driver
// see both directions of traffic
func (tx *FullHttpTransaction) Incomplete() bool {
	return false
}

func (tx *FullHttpTransaction) Path(buffer []byte) ([]byte, bool) {
	bLen := bytes.IndexByte(tx.RequestFragment, 0)
	if bLen == -1 {
		bLen = len(tx.RequestFragment)
	}
	// trim null byte + after
	b := tx.RequestFragment[:bLen]
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
func (tx *FullHttpTransaction) SetStatusCode(code uint16) {
	tx.Txn.ResponseStatusCode = code
}

func (tx *FullHttpTransaction) ResponseLastSeen() uint64 {
	return tx.Txn.ResponseLastSeen
}

func (tx *FullHttpTransaction) SetResponseLastSeen(ls uint64) {
	tx.Txn.ResponseLastSeen = ls
}

func (tx *FullHttpTransaction) RequestStarted() uint64 {
	return tx.Txn.RequestStarted
}

func (tx *FullHttpTransaction) RequestMethod() uint32 {
	return tx.Txn.RequestMethod
}

func (tx *FullHttpTransaction) SetRequestMethod(m uint32) {
	tx.Txn.RequestMethod = m
}

// --------------------------
//
// etwHttpTX interface
//

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *etwHttpTX) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

func (tx *etwHttpTX) StatusClass() int {
	return statusClass(tx.Txn.ResponseStatusCode)
}

func (tx *etwHttpTX) RequestLatency() float64 {
	return requestLatency(tx.Txn.ResponseLastSeen, tx.Txn.RequestStarted)
}

func (tx *etwHttpTX) isIPV4() bool {
	return isIPV4(&tx.Txn.Tup)
}

func (tx *etwHttpTX) SrcIPLow() uint64 {
	return srcIPLow(&tx.Txn.Tup)
}

func (tx *etwHttpTX) SrcIPHigh() uint64 {
	return srcIPHigh(&tx.Txn.Tup)
}

func (tx *etwHttpTX) SrcPort() uint16 {
	return tx.Txn.Tup.CliPort
}

func (tx *etwHttpTX) DstIPLow() uint64 {
	return dstIPLow(&tx.Txn.Tup)
}

func (tx *etwHttpTX) DstIPHigh() uint64 {
	return dstIPHigh(&tx.Txn.Tup)
}

func (tx *etwHttpTX) DstPort() uint16 {
	return tx.Txn.Tup.SrvPort
}

func (tx *etwHttpTX) Method() Method {
	return Method(tx.Txn.RequestMethod)
}

func (tx *etwHttpTX) SetRequestMethod(m uint32) {
	tx.Txn.RequestMethod = m
}

func (tx *etwHttpTX) StatusCode() uint16 {
	return tx.Txn.ResponseStatusCode
}

// Static Tags are not part of windows http transactions
func (tx *etwHttpTX) StaticTags() uint64 {
	return 0
}

// Dynamic Tags are  part of windows http transactions
func (tx *etwHttpTX) DynamicTags() []string {
	return []string{
		fmt.Sprintf("http.iis.app_pool:%v", tx.AppPool),
		fmt.Sprintf("http.iis.site:%v", tx.SiteID),
		fmt.Sprintf("http.iis.sitename:%v", tx.SiteName),
		fmt.Sprintf("service:%v", tx.AppPool),
	}
}

func (tx *etwHttpTX) String() string {
	var output strings.Builder
	output.WriteString("httpTX{")
	output.WriteString("Method: '" + tx.Method().String() + "', ")
	//output.WriteString("Fragment: '" + hex.EncodeToString(tx.RequestFragment[:]) + "', ")
	output.WriteString("\n  Fragment: '" + string(tx.RequestFragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

// Incomplete transactions does not apply to windows
func (tx *etwHttpTX) Incomplete() bool {
	return false
}

func (tx *etwHttpTX) Path(buffer []byte) ([]byte, bool) {
	bLen := bytes.IndexByte(tx.RequestFragment, 0)
	if bLen == -1 {
		bLen = len(tx.RequestFragment)
	}
	// trim null byte + after
	b := tx.RequestFragment[:bLen]
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

func (tx *etwHttpTX) SetStatusCode(code uint16) {
	tx.Txn.ResponseStatusCode = code
}

func (tx *etwHttpTX) ResponseLastSeen() uint64 {
	return tx.Txn.ResponseLastSeen
}

func (tx *etwHttpTX) SetResponseLastSeen(ls uint64) {
	tx.Txn.ResponseLastSeen = ls
}

func (tx *etwHttpTX) RequestStarted() uint64 {
	return tx.Txn.RequestStarted
}

func (tx *etwHttpTX) RequestMethod() uint32 {
	return tx.Txn.RequestMethod
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
