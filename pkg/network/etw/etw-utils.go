// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package etw

import (
	"bytes"
	"fmt"
	"net/netip"
	"reflect"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
)

/*
#include "etw.h"
*/
import "C"

// From GetUnixTimestamp() datadog-windows-filter\ddfilter\http\http_callbacks.c
// difference between windows and unix epochs in 100ns intervals
// 11644473600s * 1000ms/s * 1000us/ms * 10 intervals/us
const EPOCH_DIFFERENCE_SECS uint64 = 116444736000000000

// From HTTP_VERB enumeration (http.h)
const (
	httpVerbUnparsed uint32 = iota
	httpVerbUnknown
	httpVerbInvalid
	httpVerbOPTIONS
	httpVerbGET
	httpVerbHEAD
	httpVerbPOST
	httpVerbPUT
	httpVerbDELETE
	httpVerbTRACE
	httpVerbCONNECT
	httpVerbTRACK
	httpVerbMOVE
	httpVerbCOPY
	httpVerbPROPFIND
	httpVerbPROPPATCH
	httpVerbMKCOL
	httpVerbLOCK
	httpVerbUNLOCK
	httpVerbSEARCH
	httpVerbMaximum
)

var (
	verb2method = []transaction.Method{
		// from HTTP_VERB enumeration (http.h) (etw-http-service.go)
		//
		// looks like MS does not define verb for PATCH method
		//
		transaction.MethodUnknown, // httpVerbUnparsed uint32 = iota
		transaction.MethodUnknown, // httpVerbUnknown
		transaction.MethodUnknown, // httpVerbInvalid
		transaction.MethodOptions, // httpVerbOPTIONS
		transaction.MethodGet,     // httpVerbGET
		transaction.MethodHead,    // httpVerbHEAD
		transaction.MethodPost,    // httpVerbPOST
		transaction.MethodPut,     // httpVerbPUT
		transaction.MethodDelete,  // httpVerbDELETE
		transaction.MethodUnknown, // httpVerbTRACE
		transaction.MethodUnknown, // httpVerbCONNECT
		transaction.MethodUnknown, // httpVerbTRACK
		transaction.MethodUnknown, // httpVerbMOVE
		transaction.MethodUnknown, // httpVerbCOPY
		transaction.MethodUnknown, // httpVerbPROPFIND
		transaction.MethodUnknown, // httpVerbPROPPATCH
		transaction.MethodUnknown, // httpVerbMKCOL
		transaction.MethodUnknown, // httpVerbLOCK
		transaction.MethodUnknown, // httpVerbUNLOCK
		transaction.MethodUnknown, // httpVerbSEARCH
		transaction.MethodUnknown, // httpVerbMaximum
	}
)

func verbToMethod(verb uint32) transaction.Method {
	if verb >= httpVerbMaximum {
		return transaction.MethodUnknown
	}

	return verb2method[verb]
}

func httpVerbToStr(httVerb uint32) string {
	if httVerb >= httpVerbMaximum {
		return "<UNKNOWN>"
	}

	return [...]string{
		"Unparsed",  // httpVerbUnparsed
		"Unknown",   // httpVerbUnknown
		"Invalid",   // httpVerbInvalid
		"OPTIONS",   // httpVerbOPTIONS
		"GET",       // httpVerbGET
		"HEAD",      // httpVerbHEAD
		"POST",      // httpVerbPOST
		"PUT",       // httpVerbPUT
		"DELETE",    // httpVerbDELETE
		"TRACE",     // httpVerbTRACE
		"CONNECT",   // httpVerbCONNECT
		"TRACK",     // httpVerbTRACK
		"MOVE",      // httpVerbMOVE
		"COPY",      // httpVerbCOPY
		"PROPFIND",  // httpVerbPROPFIND
		"PROPPATCH", // httpVerbPROPPATCH
		"MKCOL",     // httpVerbMKCOL
		"LOCK",      // httpVerbLOCK
		"UNLOCK",    // httpVerbUNLOCK
		"SEARCH",    // httpVerbSEARCH
	}[httVerb]
}

func httpMethodToStr(httpMethod uint32) string {
	if httpMethod >= uint32(transaction.MethodMaximum) {
		return "<UNKNOWN>"
	}

	return [...]string{
		"UNKNOWN", // methodUnknown
		"GET",     // methodGet
		"POST",    // methodPost
		"PUT",     // methodPut
		"DELETE",  // methodDelete
		"HEAD",    // methodHead
		"PATCH",   // methodPatch
	}[httpMethod]
}

// // From
// //     datadog-agent\pkg\network\tracer\common_linux.go
// //     datadog-agent\pkg\network\tracer\offsetguess.go
// func htons(a uint16) uint16 {
// 	var arr [2]byte
// 	binary.BigEndian.PutUint16(arr[:], a)
// 	return binary.LittleEndian.Uint16(arr[:])
// }

func goBytes(data unsafe.Pointer, len C.int) []byte {
	// It could be as simple and safe as
	// 		C.GoBytes(edata, len))
	// but it copies buffer data which seems to be a waste in many
	// cases especially if it is only for a serialization. Instead
	// we make a syntetic slice which reference underlying buffer.
	// According to some measurements this approach is 10x faster
	// then built-in method

	var slice []byte
	sliceHdr := (*reflect.SliceHeader)((unsafe.Pointer(&slice)))
	sliceHdr.Cap = int(len)
	sliceHdr.Len = int(len)
	sliceHdr.Data = uintptr(data)
	return slice
}

func bytesIndexOfDoubleZero(data []byte) int {
	dataLen := len(data)
	if dataLen < 2 {
		return -1
	}

	for i := 0; i < dataLen-1; i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			return i
		}
	}

	return -1
}

// From
//    datadog-agent\pkg\util\winutil\winstrings.go
// ConvertWindowsString converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func convertWindowsString(winput []uint8) string {
	p := (*[1 << 29]uint16)(unsafe.Pointer(&winput[0]))[: len(winput)/2 : len(winput)/2]
	return windows.UTF16ToString(p)
}

func formatGuid(guid C.DDGUID) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X%02X%02X%02X%02X%02X%02X}",
		guid.Data1, guid.Data2, guid.Data3,
		guid.Data4[0], guid.Data4[1], guid.Data4[2], guid.Data4[3],
		guid.Data4[4], guid.Data4[5], guid.Data4[6], guid.Data4[7])
}

func bytesFormat(b uint64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func formatUInt(num uint64) string {
	output := strconv.FormatUint(num, 10)
	offset := 3

	for outputIdx := len(output); outputIdx > offset; {
		outputIdx -= 3
		output = output[:outputIdx] + "," + output[outputIdx:]
	}

	return output
}

// stampToTime translates FileTime to a golang time. Same as in standard packages.
// // From GetUnixTimestamp() datadog-windows-filter\ddfilter\http\http_callbacks.c
// returns timestamp in ns since unix epoch
func fileTimeToUnixTime(ft uint64) uint64 {
	return (ft - EPOCH_DIFFERENCE_SECS) * 100
}

func formatUnixTime(t uint64) string {
	tm := time.Unix(int64(t/1000000000), int64(t%1000000000))
	return tm.Format("01/02/2006 03:04:05.000000 pm")
}

func parseUnicodeString(data []byte, offset int) (val string, nextOffset int, valFound bool, foundTermZeroIdx int) {
	termZeroIdx := bytesIndexOfDoubleZero(data[offset:])
	var lenString int
	var skip int
	if termZeroIdx == 0 || termZeroIdx%2 == 1 {
		return "", -1, false, offset + termZeroIdx
	}
	if termZeroIdx == -1 {
		// wasn't null terminated.  Assume it's still a valid string though
		lenString = len(data) - offset
	} else {
		lenString = termZeroIdx
		skip = 2
	}
	val = convertWindowsString(data[offset : offset+lenString])
	nextOffset = offset + lenString + skip
	valFound = true
	foundTermZeroIdx = termZeroIdx
	return
}

func parseAsciiString(data []byte, offset int) (val string, nextOffset int, valFound bool, foundTermZeroIdx int) {
	singleZeroSlice := []byte{0}
	termZeroIdx := bytes.Index(data[offset:], singleZeroSlice)
	if termZeroIdx == -1 || termZeroIdx == 0 {
		return "", -1, false, offset + termZeroIdx
	}

	return string(data[offset : offset+termZeroIdx]), (offset + termZeroIdx + 1), true, (offset + termZeroIdx + 1)
}

func skipAsciiString(data []byte, offset int) (nextOffset int, valFound bool, foundTermZeroIdx int) {
	singleZeroSlice := []byte{0}
	termZeroIdx := bytes.Index(data[offset:], singleZeroSlice)
	if termZeroIdx == -1 || termZeroIdx == 0 {
		return -1, false, offset + termZeroIdx
	}

	return (offset + termZeroIdx + 1), true, (offset + termZeroIdx + 1)
}

func ip4format(ip [16]uint8) string {
	ipObj := netip.AddrFrom4(*(*[4]byte)(ip[:4]))
	return ipObj.String()
}

func ip6format(ip [16]uint8) string {
	ipObj := netip.AddrFrom16(ip)
	return ipObj.String()
}

func ipAndPortFromTup(tup driver.ConnTupleType, srv bool) ([16]uint8, uint16) {
	if srv {
		return tup.SrvAddr, tup.SrvPort
	} else {
		return tup.CliAddr, tup.CliPort
	}
}

func ipFormat(tup driver.ConnTupleType, srv bool) string {
	ip, port := ipAndPortFromTup(tup, srv)

	if tup.Family == 2 {
		// IPv4
		return fmt.Sprintf("%v:%v", ip4format(ip), port)
	} else if tup.Family == 23 {
		// IPv6
		return fmt.Sprintf("[%v]:%v", ip6format(ip), port)
	} else {
		// everything else
		return "<UNKNOWN>"
	}
}
