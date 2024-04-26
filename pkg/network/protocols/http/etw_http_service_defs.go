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
)

const (
	//revive:disable:var-naming Name is intended to match the Windows API name
	//revive:disable:exported
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn         = 21
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose        = 23
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnCleanup      = 24
	EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq             = 1
	EVENT_ID_HttpService_HTTPRequestTraceTaskParse               = 2
	EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver             = 3
	EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp            = 4
	EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp            = 8
	EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache        = 16
	EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified   = 17
	EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry       = 25
	EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache          = 27
	EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent            = 34
	EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete        = 10
	EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend       = 11
	EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend            = 12
	EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend            = 13
	EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError        = 14
	EVENT_ID_HttpService_HTTPRequestTraceTaskRequestRejectedArgs = 64
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
