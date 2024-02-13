// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package http

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	//revive:disable:var-naming Name is intended to match the Windows API name
	//revive:disable:exported
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn       = 0x15
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose      = 0x17
	EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq           = 0x1
	EVENT_ID_HttpService_HTTPRequestTraceTaskParse             = 0x2
	EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver           = 0x3
	EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp          = 0x4
	EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp          = 0x8
	EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache      = 0x10
	EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified = 0x11
	EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry     = 0x19
	EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache        = 0x1b
	EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent          = 0x22
	EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete      = 0xa
	EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend     = 0xb
	EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend          = 0xc
	EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend          = 0xd
	EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError      = 0xe
	//revive:enable:exported
	//revive:enable:var-naming
)

var (
	verb2method = []Method{
		// from HTTP_VERB enumeration (http.h) (etw-http-service.go)
		//
		// looks like MS does not define verb for PATCH method
		//
		MethodUnknown, // httpVerbUnparsed uint32 = iota
		MethodUnknown, // httpVerbUnknown
		MethodUnknown, // httpVerbInvalid
		MethodOptions, // httpVerbOPTIONS
		MethodGet,     // httpVerbGET
		MethodHead,    // httpVerbHEAD
		MethodPost,    // httpVerbPOST
		MethodPut,     // httpVerbPUT
		MethodDelete,  // httpVerbDELETE
		MethodUnknown, // httpVerbTRACE
		MethodUnknown, // httpVerbCONNECT
		MethodUnknown, // httpVerbTRACK
		MethodUnknown, // httpVerbMOVE
		MethodUnknown, // httpVerbCOPY
		MethodUnknown, // httpVerbPROPFIND
		MethodUnknown, // httpVerbPROPPATCH
		MethodUnknown, // httpVerbMKCOL
		MethodUnknown, // httpVerbLOCK
		MethodUnknown, // httpVerbUNLOCK
		MethodUnknown, // httpVerbSEARCH
		MethodUnknown, // httpVerbMaximum
	}
)

// VerbToMethod converts an http verb to a method
func VerbToMethod(verb uint32) Method {
	if verb >= uint32(len(verb2method)) {
		return MethodUnknown
	}

	return verb2method[verb]
}

// FormatUnixTime takes a unix timestamp and returns a human readable string
func FormatUnixTime(t uint64) string {
	tm := time.Unix(int64(t/1000000000), int64(t%1000000000))
	return tm.Format("01/02/2006 03:04:05.000000 pm")
}

// ParseUnicodeString takes a slice of bytes and converts it to a string
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
	val = winutil.ConvertWindowsString(data[offset : offset+lenString])
	nextOffset = offset + lenString + skip
	valFound = true
	foundTermZeroIdx = termZeroIdx
	return
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
