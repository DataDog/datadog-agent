// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package transaction

import (
	"encoding/binary"
	//"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"golang.org/x/sys/windows"
)

///type etwHttpTX struct {
//	httpTX
//	*etw.Http
//}

type WinHttpTransaction struct {
	Txn             driver.HttpTransactionType
	RequestFragment []byte

	// ... plus some extra
	// only valid on ETW
	AppPool string
	// <<<MORE ETW HttpService DETAILS>>>
	// We can track FULL url and few other attributes. However it will require much memory.
	// Search for <<<MORE ETW HttpService DETAILS>>> top find all places to be uncommented
	// if such tracking is desired
	//
	// Url           string
	SiteID   uint32
	SiteName string
	// HeaderLength  uint32
	// ContentLength uint32

}

// ErrLostBatch isn't a valid error in windows
var ErrLostBatch = errors.New("invalid error")

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
func (tx *WinHttpTransaction) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

func (tx *WinHttpTransaction) StatusClass() int {
	return statusClass(tx.Txn.ResponseStatusCode)
}

func (tx *WinHttpTransaction) RequestLatency() float64 {
	return requestLatency(tx.Txn.ResponseLastSeen, tx.Txn.RequestStarted)
}

func (tx *WinHttpTransaction) isIPV4() bool {
	return isIPV4(&tx.Txn.Tup)
}

func (tx *WinHttpTransaction) Method() Method {
	return Method(tx.Txn.RequestMethod)
}

func (tx *WinHttpTransaction) StatusCode() uint16 {
	return tx.Txn.ResponseStatusCode
}

// Static Tags are not part of windows driver http transactions
func (tx *WinHttpTransaction) StaticTags() uint64 {
	return 0
}

// Dynamic Tags are not part of windows driver http transactions
func (tx *WinHttpTransaction) DynamicTags() []string {
	return nil
}

func (tx *WinHttpTransaction) String() string {
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
func (tx *WinHttpTransaction) Incomplete() bool {
	return false
}

func (tx *WinHttpTransaction) Path(buffer []byte) ([]byte, bool) {
	b := tx.RequestFragment

	// b might contain a null terminator in the middle
	bLen := strlen(b[:])

	var i, j int
	for i = 0; i < bLen && b[i] != ' '; i++ {
	}

	i++

	if i >= bLen || (b[i] != '/' && b[i] != '*') {
		return nil, false
	}

	for j = i; j < bLen && b[j] != ' ' && b[j] != '?'; j++ {
	}

	// no bound check necessary here as we know we at least have '/' character
	n := copy(buffer, b[i:j])
	fullPath := j < bLen || (j == int(tx.Txn.MaxRequestFragment-1) && b[j] == ' ')
	return buffer[:n], fullPath

}
func (tx *WinHttpTransaction) SetStatusCode(code uint16) {
	tx.Txn.ResponseStatusCode = code
}

func (tx *WinHttpTransaction) ResponseLastSeen() uint64 {
	return tx.Txn.ResponseLastSeen
}

func (tx *WinHttpTransaction) SetResponseLastSeen(ls uint64) {
	tx.Txn.ResponseLastSeen = ls
}

func (tx *WinHttpTransaction) RequestStarted() uint64 {
	return tx.Txn.RequestStarted
}

func (tx *WinHttpTransaction) RequestMethod() uint32 {
	return tx.Txn.RequestMethod
}

func (tx *WinHttpTransaction) SetRequestMethod(m uint32) {
	tx.Txn.RequestMethod = m
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

func (tx *WinHttpTransaction) NewKey(path string, fullPath bool) Key {
	return Key{
		KeyTuple: KeyTuple{
			SrcIPHigh: srcIPHigh(&tx.Txn.Tup),
			SrcIPLow:  srcIPLow(&tx.Txn.Tup),
			SrcPort:   tx.Txn.Tup.CliPort,
			DstIPHigh: dstIPHigh(&tx.Txn.Tup),
			DstIPLow:  dstIPLow(&tx.Txn.Tup),
			DstPort:   tx.Txn.Tup.SrvPort,
		},
		Path: Path{
			Content:  path,
			FullPath: fullPath,
		},
		Method: tx.Method(),
	}
}

func (tx *WinHttpTransaction) NewKeyTuple() KeyTuple {
	return KeyTuple{
		SrcIPHigh: srcIPHigh(&tx.Txn.Tup),
		SrcIPLow:  srcIPLow(&tx.Txn.Tup),
		SrcPort:   tx.Txn.Tup.CliPort,
	}
}
