// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

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
)

/*
#include "etw.h"
*/
import "C"

// Copied from github.com\DataDog\datadog-agent\pkg\network\http\http_stats.go
// Note: cannot refer to it due to package circularity
//
//nolint:deadcode
const (
	methodUnknown uint32 = iota
	methodGet
	methodPost
	methodPut
	methodDelete
	methodHead
	methodOptions
	methodPatch
	methodMaximum
)

// From HTTP_VERB enumeration (http.h)
//
//nolint:deadcode
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
	verb2method = []uint32{
		// from HTTP_VERB enumeration (http.h) (etw-http-service.go)
		//
		// looks like MS does not define verb for PATCH method
		//
		methodUnknown, // httpVerbUnparsed uint32 = iota
		methodUnknown, // httpVerbUnknown
		methodUnknown, // httpVerbInvalid
		methodOptions, // httpVerbOPTIONS
		methodGet,     // httpVerbGET
		methodHead,    // httpVerbHEAD
		methodPost,    // httpVerbPOST
		methodPut,     // httpVerbPUT
		methodDelete,  // httpVerbDELETE
		methodUnknown, // httpVerbTRACE
		methodUnknown, // httpVerbCONNECT
		methodUnknown, // httpVerbTRACK
		methodUnknown, // httpVerbMOVE
		methodUnknown, // httpVerbCOPY
		methodUnknown, // httpVerbPROPFIND
		methodUnknown, // httpVerbPROPPATCH
		methodUnknown, // httpVerbMKCOL
		methodUnknown, // httpVerbLOCK
		methodUnknown, // httpVerbUNLOCK
		methodUnknown, // httpVerbSEARCH
		methodUnknown, // httpVerbMaximum
	}
)

// VerbToMethod converts an http verb to a method
func VerbToMethod(verb uint32) uint32 {
	if verb >= httpVerbMaximum {
		return methodUnknown
	}

	return verb2method[verb]
}

// HttpVerbToStr converts the integer verb type to a human readable string
func HttpVerbToStr(httVerb uint32) string {
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

// HttpMethodToStr converts the integer representation of the method to string
func HttpMethodToStr(httpMethod uint32) string {
	if httpMethod >= methodMaximum {
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

func GoBytes(data unsafe.Pointer, len int) []byte {
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
//
//	datadog-agent\pkg\util\winutil\winstrings.go
//
// ConvertWindowsString converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func convertWindowsString(winput []uint8) string {
	p := (*[1 << 29]uint16)(unsafe.Pointer(&winput[0]))[: len(winput)/2 : len(winput)/2]
	return windows.UTF16ToString(p)
}

// FormatGuid converts a guid structure to a go string
func FormatGuid(guid DDGUID) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X%02X%02X%02X%02X%02X%02X}",
		guid.Data1, guid.Data2, guid.Data3,
		guid.Data4[0], guid.Data4[1], guid.Data4[2], guid.Data4[3],
		guid.Data4[4], guid.Data4[5], guid.Data4[6], guid.Data4[7])
}

// BytesFormat converts a uint64 into a nice string
func BytesFormat(b uint64) string {
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

// FormatUInt converts a uint64 to a string with commas in every 3 orders of magnitude.
func FormatUInt(num uint64) string {
	output := strconv.FormatUint(num, 10)
	offset := 3

	for outputIdx := len(output); outputIdx > offset; {
		outputIdx -= 3
		output = output[:outputIdx] + "," + output[outputIdx:]
	}

	return output
}

// FormatUnixTime takes a unix timestamp and returns a human readable string
func FormatUnixTime(t uint64) string {
	tm := time.Unix(int64(t/1000000000), int64(t%1000000000))
	return tm.Format("01/02/2006 03:04:05.000000 pm")
}

// ParuseUnicodeString takes a slice of bytes and converts it to a string
func ParseUnicodeString(data []byte, offset int) (val string, nextOffset int, valFound bool, foundTermZeroIdx int) {
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

//nolint:deadcode
func parseAsciiString(data []byte, offset int) (val string, nextOffset int, valFound bool, foundTermZeroIdx int) {
	singleZeroSlice := []byte{0}
	termZeroIdx := bytes.Index(data[offset:], singleZeroSlice)
	if termZeroIdx == -1 || termZeroIdx == 0 {
		return "", -1, false, offset + termZeroIdx
	}

	return string(data[offset : offset+termZeroIdx]), (offset + termZeroIdx + 1), true, (offset + termZeroIdx + 1)
}

//nolint:deadcode
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

func ipAndPortFromTup(tup driver.ConnTupleType, local bool) ([16]uint8, uint16) {
	if local {
		return tup.LocalAddr, tup.LocalPort
	} else {
		return tup.RemoteAddr, tup.RemotePort
	}
}

// IpFormat takes a binary ip representation and returns a string type
func IpFormat(tup driver.ConnTupleType, local bool) string {
	ip, port := ipAndPortFromTup(tup, local)

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
