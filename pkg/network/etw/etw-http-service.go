// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package etw

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/windows"
	"inet.af/netaddr"
)

/*
	/////////////////////////////////////////////////////////////////////////////////////////
	Before understand the flow and the code I recommend to install Windows SDK with Performance
	Analisis enabled. Experiment using following approach

    1. Capture under different HTTP load and profile scenario and save it to a file (http.etl)
	   a.xperf -on PROC_THREAD+LOADER+Base -start httptrace -on Microsoft-Windows-HttpService
       b.  ... initiate http connections using various profiles
       c. xperf -stop -stop httptrace -d http.etl

	2. Load into Windows Performance Analyzer by double click on http.etl file

	3. Display Event window and filter only to Microsoft-Windows-HttpService events
	  a. Double click on "System-Activity/Generic Events" on a left to open Generic Events
	     Windows.
	  b. Select Microsoft-Windows-HttpService in the Series windows, right mouse button click
	     on it and select the "Filter to selection" menu item.

	4. Sort HTTP events in time ascending order and make few other column choices to maximize
	   the screen
	  a. Right button click in the column bar and select the "Open View Editor ..." menu
	  b. Drag "DateTime(local)" before "Task Name"
	  c. Drag "etw:ActivityId" after "DateTime" name
	  d. Drag "etw:Related ActivityId" after "etw:ActivityId" name
	  e. Uncheck "Provider Name"
	  f. Uncheck "Event Name"
	  g. Uncheck "cpu"

	/////////////////////////////////////////////////////////////////////////////////////////
    HTTP and App Pool info detection performance overhead

	To detect HTTP and App Pool information I had to activate Microsoft-Windows-HttpService
	ETW source and from "atomic" ETW events create synthetic HTTP events. It seems to be
	 working well but its performance impact is not negligent.

	Roughly speaking, in terms of overhead, there are 3 distinct activities used to generate
	a HTTP event. Here they are with their respective overhead:

	   * [~45% of total overhead] ETW Data Transfer from Kernel.
	       Windows implicitly transfers ETW event data blobs about HTTP activity from kernel
		   to our go process pace and invoking our ETW event handler callback.

       * [~35% of total overhead] ETW Data Parsing.
	       Our Callback is parsing HTTP strings, numbers and TCPIP structs from the passed
		   from kernel ETW event data blobs.

	   * [~20% of total overhead] Parsed Data Storage and Correlation.
	       Parsed data needs to be stored in few maps and correlated to eventually
		   "manufacture" a complete HTTP event (and store it to for the final consumption).

	On a 16 CPU machine collecting 3k per second HTTP events via Microsoft-Windows-HttpService
	ETW source costs 0.7%-1% of CPU usage.

   On a 16 CPU machine collecting 15k per second HTTP events via Microsoft-Windows-HttpService
   ETW source costs 4-5% of CPU usage.  During 5 minutes of sustained 15k per second HTTP request
   loads:
      * 9,000,000 HTTP requests had been processed
      * 36,000,000 ETW events had been reported (9,000,000 events were not "interesting" and
	    were not processed).
      * 2.4 Gb of data transferred to user mode and had to be parsed and correlated.

    Most likely the cost of HTTP and App Pool detection will be slightly higher after I integrate
	it into system-probe due to additional correlation or correlations. In addition I did not
	count CPU cost at the source (HTTP.sys driver) and ETW infrastructure (outside of 45% of overhead)
	which certainly exists but I am not sure how to measure that. On the other hand I have been
	trying to code in an efficient manner and perhaps there is room for further optimization (although
	almost half of the overhead cannot be optimized).

	/////////////////////////////////////////////////////////////////////////////////////////
	Flows

	1. HTTP transactions events are always in the scope of
		HTTPConnectionTraceTaskConnConn   21 [Local & Remote IP/Ports]
		HTTPConnectionTraceTaskConnClose  23


	2. HTTP Req/Resp (the same ActivityID)
	   a. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
	      HTTPRequestTraceTaskParse          2     [verb, url]
	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
		  HTTPRequestTraceTaskFastResp       8     [statusCode, verb, headerLen, cachePolicy]
		  HTTPRequestTraceTaskFastSend      12     [httpStatus]

		  or

	   b. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
	      HTTPRequestTraceTaskParse          2     [verb, url]
	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
		  HTTPRequestTraceTaskFastResp       4     [statusCode, verb, headerLen, cachePolicy = 0]
		  HTTPRequestTraceTaskSendComplete  10     [httpStatus]

		  or

	   c. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
	      HTTPRequestTraceTaskParse          2     [verb, url]
	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
		  HTTPRequestTraceTaskFastResp       4     [statusCode, verb, headerLen, cachePolicy=1]
		  HTTPRequestTraceTaskSrvdFrmCache  16     [site, bytesSent]
		  HTTPRequestTraceTaskCachedAndSend 11     [httpStatus]

		  or

	   d. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
	      HTTPRequestTraceTaskParse          2     [verb, url]
		  HTTPRequestTraceTaskSrvdFrmCache  16     [site, bytesSent]

	3. HTTP Cache
	    HTTPCacheTraceTaskAddedCacheEntry   25     [uri, statusCode, verb, headerLength, contentLength] [Correlated to http req/resp by url]
		HTTPCacheTraceTaskFlushedCache      27     [uri, statusCode, verb, headerLength, contentLength]
*/

/*
#include "./c/etw.h"
#include "./c/etw-provider.h"
*/
import "C"

const (
	OutputNone int = iota
	OutpuSummary
	OutputVerbose
	OutputVeryVerbose
)

var (
	OutputLevel      int    = OutputNone
	subscriptionName string = "dd-network-http-service"
)

// Aggregate
type HttpReqResp struct {
	// time
	reqTime  time.Time
	respTime time.Time

	// http
	appPool       string
	siteID        uint32
	verb          string
	url           string
	statusCode    uint16
	headerLength  uint32
	contentLength uint32

	fromCache bool
}

type Conn struct {
	// conntuple
	local      netaddr.IP
	remote     netaddr.IP
	localPort  uint16
	remotePort uint16

	// time
	connectedTime    time.Time
	disconnectedTime time.Time

	// http
	httpPendingBackLinks map[C.DDGUID]struct{}
	http                 []HttpReqResp
}

type HttpReqRespWithConnLink struct {
	connActivityId C.DDGUID

	http HttpReqResp
}

type HttpReqRespWithCacheInfo struct {
	statusCode     uint16
	verb           string
	headerLength   uint32
	contentLength  uint32
	expirationTime uint64
	reqRespBound   bool

	http HttpReqResp
}

// From HTTP_VERB enumeration (http.h)
const (
	HttpVerbUnparsed uint32 = iota
	HttpVerbUnknown
	HttpVerbInvalid
	HttpVerbOPTIONS
	HttpVerbGET
	HttpVerbHEAD
	HttpVerbPOST
	HttpVerbPUT
	HttpVerbDELETE
	HttpVerbTRACE
	HttpVerbCONNECT
	HttpVerbTRACK
	HttpVerbMOVE
	HttpVerbCOPY
	HttpVerbPROPFIND
	HttpVerbPROPPATCH
	HttpVerbMKCOL
	HttpVerbLOCK
	HttpVerbUNLOCK
	HttpVerbSEARCH
	HttpVerbMaximum
)

var (
	connOpened       = make(map[C.DDGUID]*Conn)
	reqResp2openConn = make(map[C.DDGUID]*HttpReqRespWithConnLink)
	connClosed       = make([]*Conn, 100)
	sysCache         = make(map[string]*HttpReqRespWithCacheInfo, 100)

	summaryCount          uint64 = 0
	eventCount            uint64 = 0
	servedFromCache       uint64 = 0
	completedRequestCount uint64 = 0
	missedConnectionCount uint64 = 0
	missedCacheCount      uint64 = 0
	parsingErrorCount     uint64 = 0
	notHandledEventsCount uint64 = 0

	transferedETWDataLength uint64 = 0

	lastSummaryTime time.Time = time.Now()
)

// ============================================
//
// U t i l i t i e s
//

func HttpVerbToStr(httVerb uint32) string {
	if httVerb >= HttpVerbMaximum {
		return "<UNKNOWN>"
	}

	return [...]string{
		"Unparsed",  // HttpVerbUnparsed
		"Unknown",   // HttpVerbUnknown
		"Invalid",   // HttpVerbInvalid
		"OPTIONS",   // HttpVerbOPTIONS
		"GET",       // HttpVerbGET
		"HEAD",      // HttpVerbHEAD
		"POST",      // HttpVerbPOST
		"PUT",       // HttpVerbPUT
		"DELETE",    // HttpVerbDELETE
		"TRACE",     // HttpVerbTRACE
		"CONNECT",   // HttpVerbCONNECT
		"TRACK",     // HttpVerbTRACK
		"MOVE",      // HttpVerbMOVE
		"COPY",      // HttpVerbCOPY
		"PROPFIND",  // HttpVerbPROPFIND
		"PROPPATCH", // HttpVerbPROPPATCH
		"MKCOL",     // HttpVerbMKCOL
		"LOCK",      // HttpVerbLOCK
		"UNLOCK",    // HttpVerbUNLOCK
		"SEARCH",    // HttpVerbSEARCH
		"Maximum",   // HttpVerbMaximum
	}[httVerb]
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
func fileTimeToGoTime(fileTime C.uint64_t) time.Time {
	ft := windows.Filetime{HighDateTime: uint32(fileTime >> 32), LowDateTime: uint32(fileTime & math.MaxUint32)}
	return time.Unix(0, ft.Nanoseconds())
}

func parseUnicodeString(data []byte, offset int) (val string, nextOffset int, valFound bool, foundTermZeroIdx int) {
	termZeroIdx := bytesIndexOfDoubleZero(data[offset:])
	if termZeroIdx == -1 || termZeroIdx == 0 || termZeroIdx%2 == 1 {
		return "", -1, false, offset + termZeroIdx
	}

	return convertWindowsString(data[offset : offset+termZeroIdx]), (offset + termZeroIdx + 2), true, (offset + termZeroIdx)
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

func completeReqRespTracking(eventInfo *C.DD_ETW_EVENT_INFO, reqRespAndLink *HttpReqRespWithConnLink) {

	// Get connection
	conn, connFound := connOpened[reqRespAndLink.connActivityId]
	if !connFound {
		missedConnectionCount++

		// No connection, no potint to keep it longer inthe pending HttpReqRespMap
		delete(reqResp2openConn, eventInfo.event.activityId)

		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskFastResp failed to find connection object\n\n",
				formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId))
		}
		return
	}

	// Time
	reqRespAndLink.http.respTime = fileTimeToGoTime(eventInfo.event.timeStamp)

	// lenlen
	// // // HttpReqResp is completed, move it to Connection and clean it up
	// // conn.http = append(conn.http, reqRespAndLink.http)

	delete(reqResp2openConn, eventInfo.event.activityId)
	delete(conn.httpPendingBackLinks, eventInfo.event.activityId)

	completedRequestCount++

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  CompletedReq:   %v\n", completedRequestCount)
		fmt.Printf("  Connected:      %v\n", conn.connectedTime.Format(time.StampMicro))
		fmt.Printf("  Requested:      %v\n", reqRespAndLink.http.reqTime.Format(time.StampMicro))
		fmt.Printf("  Responded:      %v\n", reqRespAndLink.http.respTime.Format(time.StampMicro))
		fmt.Printf("  ConnActivityId: %v\n", formatGuid(reqRespAndLink.connActivityId))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		if connFound {
			fmt.Printf("  Local:          %v:%v\n", conn.local.String(), conn.localPort)
			fmt.Printf("  Remote:         %v:%v\n", conn.remote.String(), conn.remotePort)
		}
		fmt.Printf("  Cached:         %v\n", reqRespAndLink.http.fromCache)
		fmt.Printf("  AppPool:        %v\n", reqRespAndLink.http.appPool)
		fmt.Printf("  Url:            %v\n", reqRespAndLink.http.url)
		fmt.Printf("  Verb:           %v\n", reqRespAndLink.http.verb)
		fmt.Printf("  StatusCode:     %v\n", reqRespAndLink.http.statusCode)
		fmt.Printf("  HeaderLength:   %v\n", reqRespAndLink.http.headerLength)
		fmt.Printf("  ContentLength:  %v\n", reqRespAndLink.http.contentLength)
		fmt.Printf("\n")
	} else if OutputLevel == OutputVerbose {
		fmt.Printf("%v. %v L[%v:%v], R[%v:%v], P[%v], C[%v], V[%v], H[%v], B[%v], U[%v]\n",
			completedRequestCount,
			reqRespAndLink.http.reqTime.Format(time.StampMicro),
			conn.local.String(), conn.localPort,
			conn.remote.String(), conn.remotePort,
			reqRespAndLink.http.appPool,
			reqRespAndLink.http.statusCode,
			reqRespAndLink.http.verb,
			reqRespAndLink.http.headerLength,
			reqRespAndLink.http.contentLength,
			reqRespAndLink.http.url)
	}
}

// ============================================
//
// E T W    E v e n t s   H a n d l e r s
//

// -----------------------------------------------------------
// HttpService ETW Event #21 (HTTPConnectionTraceTaskConnConn)
//
func httpCallbackOnHTTPConnectionTraceTaskConnConn(eventInfo *C.DD_ETW_EVENT_INFO) {
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPConnectionTraceTaskConnConn event (id:21, seq:%v)\n", eventCount)
	}

	//  typedef struct _EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP4
	//  {
	//  	0:  uint64_t connectionObj;
	//  	8:  uint32_t localAddrLength;
	//  	12: uint16_t localSinFamily;
	//  	14: uint16_t localPort;          // hton
	//  	16: uint32_t localIpAddress;
	//  	20: uint64_t localZeroPad;
	//  	28: uint32_t remoteAddrLength;
	//  	32: uint16_t remoteSinFamily;
	//  	34: uint16_t remotePort;         // hton
	//  	36: uint32_t remoteIpAddress;
	//  	40: uint64_t remoteZeroPad;
	//      48:
	//  } EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP4;
	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Check for size
	if eventInfo.event.userDataLength < 48 {
		fmt.Printf("*** Error: User data length for EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn is too small %v\n\n", uintptr(eventInfo.event.userDataLength))
		return
	}

	var conn Conn

	// Local and remote ipaddress and port
	conn.localPort = binary.BigEndian.Uint16(userData[14:16])
	conn.local = netaddr.IPFrom4(*(*[4]byte)(userData[16:20]))
	conn.remotePort = binary.BigEndian.Uint16(userData[34:36])
	conn.remote = netaddr.IPFrom4(*(*[4]byte)(userData[36:40]))

	// Time
	conn.connectedTime = fileTimeToGoTime(eventInfo.event.timeStamp)

	// Http
	conn.http = make([]HttpReqResp, 10)
	conn.httpPendingBackLinks = make(map[C.DDGUID]struct{}, 10)

	// Save to the map
	connOpened[eventInfo.event.activityId] = &conn

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Time:           %v\n", conn.connectedTime.Format(time.StampMicro))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  Local:          %v:%v\n", conn.local.String(), conn.localPort)
		fmt.Printf("  Remote:         %v:%v\n", conn.remote.String(), conn.remotePort)
		fmt.Printf("\n")
	}
}

// -------------------------------------------------------------
// HttpService ETW Event #23 (HTTPConnectionTraceTaskConnClose)
//
func httpCallbackOnHTTPConnectionTraceTaskConnClose(eventInfo *C.DD_ETW_EVENT_INFO) {
	// output details
	conn, found := connOpened[eventInfo.event.activityId]
	if found {
		// move it to close connection
		conn.disconnectedTime = fileTimeToGoTime(eventInfo.event.timeStamp)
		connClosed = append(connClosed, conn)
		delete(connOpened, eventInfo.event.activityId)

		// Clean pending reqResp2openConn
		for httpReqRespActivityId := range conn.httpPendingBackLinks {
			delete(reqResp2openConn, httpReqRespActivityId)
		}

	} else {
		missedConnectionCount++
	}

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPConnectionTraceTaskConnClose event (id:23, seq:%v)\n", eventCount)
		if found {
			fmt.Printf("  ActivityId: %v, Local[%v:%v], Remote[%v:%v])\n",
				formatGuid(eventInfo.event.activityId),
				conn.local.String(), conn.localPort, conn.remote.String(), conn.remotePort)
		} else {
			fmt.Printf("  ActivityId: %v not found\n", formatGuid(eventInfo.event.activityId))
		}
		fmt.Printf("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #1 (HTTPRequestTraceTaskRecvReq)
//
func httpCallbackOnHTTPRequestTraceTaskRecvReq(eventInfo *C.DD_ETW_EVENT_INFO) {
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPRequestTraceTaskRecvReq event (id:1, seq:%v)\n", eventCount)
	}

	// 	typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4
	// 	{
	// 		0:  uint64_t requestId;
	// 		8:  uint64_t connectionId;
	//      16: uint32_t remoteAddrLength;
	//      20: uint16_t remoteSinFamily;
	//      22: uint16_t remotePort;
	// 		24: uint32_t remoteIpAddress;
	//      28: uint64_t remoteZeroPad;
	//      36
	// 	} EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4;
	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Check for size
	if eventInfo.event.userDataLength < 36 {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4 is too small %v\n\n",
			formatGuid(eventInfo.event.activityId), uintptr(eventInfo.event.userDataLength))
		return
	}

	// related activityid
	if eventInfo.relatedActivityId == nil {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. HTTPRequestTraceTaskRecvReq event should have a reference to related activity id\n\n",
			formatGuid(eventInfo.event.activityId))
		return
	}

	conn, connFound := connOpened[eventInfo.event.activityId]
	if !connFound {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. Releated ActivityId:%v. HTTPRequestTraceTaskRecvReq failed to find connection object\n",
				formatGuid(eventInfo.event.activityId), formatGuid(*eventInfo.relatedActivityId))
		}
		return
	}

	// Extra output and validation
	if OutputLevel == OutputVeryVerbose {
		remotePort := binary.BigEndian.Uint16(userData[22:24])
		remote := netaddr.IPFrom4(*(*[4]byte)(userData[24:28]))
		if remote != conn.remote || remotePort != conn.remotePort {
			parsingErrorCount++
			fmt.Printf("Warning: ActivityId:%v. Releated ActivityId:%v. Recv remote %v:%v do not match Conn remote %v:%v\n",
				formatGuid(eventInfo.event.activityId), formatGuid(*eventInfo.relatedActivityId),
				remote, remotePort, conn.remote, conn.remotePort)
		}
	}

	// Initialize ReqResp and Conn Link
	reqRespAndLink := &HttpReqRespWithConnLink{}
	reqRespAndLink.connActivityId = eventInfo.event.activityId
	reqRespAndLink.http.reqTime = fileTimeToGoTime(eventInfo.event.timeStamp)

	// Save Req/Resp Conn Link and back reference to it
	reqResp2openConn[*eventInfo.relatedActivityId] = reqRespAndLink
	var dummy struct{}
	conn.httpPendingBackLinks[*eventInfo.relatedActivityId] = dummy

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Time:           %v\n", reqRespAndLink.http.reqTime.Format(time.StampMicro))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  RelActivityId:  %v\n", formatGuid(*eventInfo.relatedActivityId))
		if connFound {
			fmt.Printf("  Local:          %v:%v\n", conn.local.String(), conn.localPort)
			fmt.Printf("  Remote:         %v:%v\n", conn.remote.String(), conn.remotePort)
		}
		fmt.Printf("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #2 (HTTPRequestTraceTaskParse)
//
func httpCallbackOnHTTPRequestTraceTaskParse(eventInfo *C.DD_ETW_EVENT_INFO) {
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPRequestTraceTaskParse event (id:2, seq:%v)\n", eventCount)
	}

	// typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskParse
	// {
	// 	    0:  uint64_t requestObj;
	// 	    8:  uint32_t httpVerb;
	// 	    12: unint8_t url;           // Unicode wide char zero terminating string
	// } EVENT_PARAM_HttpService_HTTPRequestTraceTaskParse;
	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Check for size
	if eventInfo.event.userDataLength < 14 {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. User data length for HTTPRequestTraceTaskParse is too small %v\n\n",
			formatGuid(eventInfo.event.activityId), uintptr(eventInfo.event.userDataLength))
		return
	}

	// Get req/resp conn link
	reqRespAndLink, found := reqResp2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskParse failed to find connection ActivityID\n\n", formatGuid(eventInfo.event.activityId))
		return
	}

	// Verb (in future we can cast number to)
	httpVerb := binary.LittleEndian.Uint32(userData[8:12])
	reqRespAndLink.http.verb = HttpVerbToStr(httpVerb)

	// Parse Url
	urlOffset := 12
	url, _, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. HTTPRequestTraceTaskParse could not find terminating zero for Url. termZeroIdx=%v\n\n",
			formatGuid(eventInfo.event.activityId), urlTermZeroIdx)

		// If problem stop tracking this
		delete(reqResp2openConn, eventInfo.event.activityId)
		return
	}

	reqRespAndLink.http.url = url

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  Url:            %v\n", reqRespAndLink.http.url)
		fmt.Printf("  Verb:           %v\n", reqRespAndLink.http.verb)
		fmt.Printf("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #3 (HTTPRequestTraceTaskDeliver)
//
func httpCallbackOnHTTPRequestTraceTaskDeliver(eventInfo *C.DD_ETW_EVENT_INFO) {
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPRequestTraceTaskDeliver event (id:3, seq:%v)\n", eventCount)
	}

	// 	typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskDeliver
	// 	{
	// 		0:  uint64_t requestObj;
	// 		8:  uint64_t requestId;
	// 		16: uint32_t siteId;
	// 		20: uint8_t  requestQueueName[xxx];  // Unicode zero terminating string
	// 	        uint8_t  url[xxx];               // Unicode zero terminating string
	// 	        uint32_t status;
	// 	} EVENT_PARAM_HttpService_HTTPRequestTraceTaskDeliver;
	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Check for size
	if eventInfo.event.userDataLength < 24 {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskDeliver is too small %v\n\n",
			formatGuid(eventInfo.event.activityId), uintptr(eventInfo.event.userDataLength))
		return
	}

	// Get req/resp conn link
	reqRespAndLink, found := reqResp2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskDeliver failed to find connection ActivityID\n\n", formatGuid(eventInfo.event.activityId))
		}
		return
	}

	// Extra output
	conn, connFound := connOpened[reqRespAndLink.connActivityId]
	if !connFound {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver failed to find connection object\n",
				formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId))
		}

		// If no connection found then stop tracking
		delete(reqResp2openConn, eventInfo.event.activityId)
		return
	}

	// Parse RequestQueueName
	appPoolOffset := 20
	appPool, urlOffset, appPoolFound, appPoolTermZeroIdx := parseUnicodeString(userData, appPoolOffset)
	if !appPoolFound {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId), appPoolTermZeroIdx)

		// If problem stop tracking this
		delete(reqResp2openConn, eventInfo.event.activityId)
		return
	}

	reqRespAndLink.http.appPool = appPool

	// Parse url
	if urlOffset > len(userData) {
		parsingErrorCount++

		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find begining of Url\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId))

		// If problem stop tracking this
		delete(reqResp2openConn, eventInfo.event.activityId)
		return
	}

	// Parse url (skip it because it is already captured httpCallbackOnHTTPRequestTraceTaskParse already)
	// Previous implementation (we can use it in future if configured to cross-validation)
	//    url, _, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	//    reqRespAndLink.http.url = url
	//    if !urlFound {
	//    	parsingErrorCount++
	//    	fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find terminating zero for url. termZeroIdx=%v\n\n",
	//    		formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId), urlTermZeroIdx)
	//
	//    	// If problem stop tracking this
	//    	delete(reqResp2openConn, eventInfo.event.activityId)
	//    	return
	//    }
	//    reqRespAndLink.http.url = url

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  ConnActivityId: %v\n", formatGuid(reqRespAndLink.connActivityId))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  AppPool:        %v\n", reqRespAndLink.http.appPool)
		fmt.Printf("  Url:            %v\n", reqRespAndLink.http.url)
		if connFound {
			fmt.Printf("  Local:          %v:%v\n", conn.local.String(), conn.localPort)
			fmt.Printf("  Remote:         %v:%v\n", conn.remote.String(), conn.remotePort)
		}
		fmt.Printf("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #4 (HTTPRequestTraceTaskFastResp, HTTPRequestTraceTaskRecvResp)
//
func httpCallbackOnHTTPRequestTraceTaskRecvResp(eventInfo *C.DD_ETW_EVENT_INFO) {
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPRequestTraceTaskXxxResp event (id:%v, seq:%v)\n", eventInfo.event.id, eventCount)
	}

	// 	typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvResp
	// 	{
	// 		0:  uint64_t  requestId;
	// 		8:  uint64_t  connectionId;
	// 		16: uint16_t  statusCode;
	// 		18: char      verb[1];      // ASCII zero terminating string string
	// 	        uint32    headerLength
	//          uint16_t  entityChunkCount
	//          uint32_t  cachePolicy
	// 	} EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvResp;

	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Check for size
	if eventInfo.event.userDataLength < 24 {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskXxxResp is too small %v\n\n",
			formatGuid(eventInfo.event.activityId), uintptr(eventInfo.event.userDataLength))
		return
	}

	// Get req/resp conn link
	reqRespAndLink, found := reqResp2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskXxxResp failed to find connection ActivityID\n\n",
				formatGuid(eventInfo.event.activityId))
		}
		return
	}
	reqRespAndLink.http.statusCode = binary.LittleEndian.Uint16(userData[16:18])

	// Parse Verb (just skip it, we already get verb string value from int in httpCallbackOnHTTPRequestTraceTaskParse)
	// Previous implementation (Previous implementation (we can use it in future if configured to cross-validation)
	//     verb, headerSizeOffset, verbFound, verbTermZeroIdx := parseAsciiString(userData, verbOffset)
	//     reqRespAndLink.http.verb = verb
	verbOffset := 18
	headerSizeOffset, verbFound, verbTermZeroIdx := skipAsciiString(userData, verbOffset)
	if !verbFound {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskXxxResp could not find terminating zero for Verb. termZeroIdx=%v\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId), verbTermZeroIdx)
		return
	}

	// Parse headerLength (space for 32bit number)
	if (headerSizeOffset + 4) > len(userData) {
		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskXxxResp Not enough space for HeaderLength. userDataSize=%v, parsedDataSize=%v\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(reqRespAndLink.connActivityId), len(userData), (headerSizeOffset + 4))
		return
	}

	reqRespAndLink.http.headerLength = binary.LittleEndian.Uint32(userData[headerSizeOffset:])

	reqRespAndLink.http.fromCache = false
}

// -----------------------------------------------------------
// HttpService ETW Event #16-17 (HTTPRequestTraceTaskSrvdFrmCache, HTTPRequestTraceTaskCachedNotModified)
//
func httpCallbackOnHTTPRequestTraceTaskSrvdFrmCache(eventInfo *C.DD_ETW_EVENT_INFO) {

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPRequestTraceTaskDeliver event (id:%v, seq:%v)\n", eventInfo.event.id, eventCount)
	}

	// typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache
	// {
	// 	   0:  uint64_t requestObj;
	// 	   8:  uint32_t SiteId;
	// 	   12: uint32_t bytesSent;
	// } EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache;

	// userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Check for size
	if eventInfo.event.userDataLength < 12 {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache is too small %v\n\n",
			formatGuid(eventInfo.event.activityId), uintptr(eventInfo.event.userDataLength))
		return
	}

	// Get req/resp conn link
	reqRespAndLink, found := reqResp2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskSrvdFrmCache failed to find connection ActivityID\n\n",
				formatGuid(eventInfo.event.activityId))
		}
		return
	}

	// Get from cache or set to the sysCache
	httpReqCacheEntry, found := sysCache[reqRespAndLink.http.url]
	if !found {
		fmt.Printf("Warning: HTTPRequestTraceTaskSrvdFrmCache failed to find HTTP cache entry by url %v\n\n", reqRespAndLink.http.url)

		// If problem stop tracking this
		if OutputLevel == OutputVeryVerbose {
			delete(reqResp2openConn, eventInfo.event.activityId)
		}
		return
	}

	// Log the findings
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Cache entry for %v is found\n", reqRespAndLink.http.url)
		if httpReqCacheEntry.reqRespBound {
			fmt.Printf("  Completing reqResp tracking\n")
		} else {
			fmt.Printf("  Updating cache entry via current http request\n")
		}
		fmt.Printf("\n")
	}

	if httpReqCacheEntry.reqRespBound {
		// Get from cache and complete reqResp tracking
		reqRespAndLink.http = httpReqCacheEntry.http
		reqRespAndLink.http.fromCache = true
		reqRespAndLink.http.appPool = httpReqCacheEntry.http.appPool
		reqRespAndLink.http.siteID = httpReqCacheEntry.http.siteID
		reqRespAndLink.http.statusCode = httpReqCacheEntry.http.statusCode

		completeReqRespTracking(eventInfo, reqRespAndLink)
		servedFromCache++
	} else {
		// Set to cache
		httpReqCacheEntry.reqRespBound = true
		httpReqCacheEntry.http = reqRespAndLink.http
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #25 (HTTPCacheTraceTaskAddedCacheEntry)
//
func httpCallbackOnHTTPCacheTraceTaskAddedCacheEntry(eventInfo *C.DD_ETW_EVENT_INFO) {

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPCacheTraceTaskAddedCacheEntry event (id:%v, seq:%v)\n", eventInfo.event.id, eventCount)
	}

	// typedef struct _EVENT_PARAM_HttpService_HTTPCacheTraceTaskAddedCacheEntry
	// {
	//      	uint8_t  url[1];         // Unicode wide char zero terminating string
	//      //  uint16_t statusCode;
	//      //  uint8_t  verb[1];        // ASCII wide char zero terminating string
	//      //  uint32_t headerLength;
	//      //  uint32_t contentLength;
	//      //  uint64_t expirationTime;
	// } EVENT_PARAM_HttpService_HTTPCacheTraceTaskAddedCacheEntry;

	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	httpReqCacheEntry := &HttpReqRespWithCacheInfo{}

	// Parse Url
	urlOffset := 0
	url, statusCodeOffset, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n", urlTermZeroIdx)
		return
	}

	// Status code
	httpReqCacheEntry.statusCode = binary.LittleEndian.Uint16(userData[statusCodeOffset : statusCodeOffset+2])

	// Parse Verb
	verbOffset := statusCodeOffset + 2
	verb, headerSizeOffset, verbFound, verbTermZeroIdx := parseAsciiString(userData, verbOffset)
	if !verbFound {
		parsingErrorCount++
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry could not find terminating zero for Verb. termZeroIdx=%v\n\n", verbTermZeroIdx)
		return
	}
	httpReqCacheEntry.verb = verb

	// Parse headerLength (space for 32bit number)
	if (headerSizeOffset + 4) > len(userData) {
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry Not enough space for HeaderLength. userDataSize=%v, parsedDataSize=%v\n\n",
			len(userData), (headerSizeOffset + 4))
		return
	}
	httpReqCacheEntry.headerLength = binary.LittleEndian.Uint32(userData[headerSizeOffset:])

	// Parse contentLength (space for 32bit number)
	contentLengthOffset := headerSizeOffset + 4
	if (contentLengthOffset + 4) > len(userData) {
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry Not enough space for contentLengthOffset. userDataSize=%v, parsedDataSize=%v\n\n",
			len(userData), (contentLengthOffset + 4))
		return
	}
	httpReqCacheEntry.contentLength = binary.LittleEndian.Uint32(userData[contentLengthOffset:])

	httpReqCacheEntry.reqRespBound = false

	// Save it to sysCache
	sysCache[url] = httpReqCacheEntry

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Url:            %v\n", url)
		fmt.Printf("  StatusCode:     %v\n", httpReqCacheEntry.statusCode)
		fmt.Printf("  Verb:           %v\n", httpReqCacheEntry.verb)
		fmt.Printf("  HeaderLength:   %v\n", httpReqCacheEntry.headerLength)
		fmt.Printf("  ContentLength:  %v\n", httpReqCacheEntry.contentLength)
		fmt.Printf("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #26 (HTTPCacheTraceTaskFlushedCache)
//
func httpCallbackOnHTTPCacheTraceTaskFlushedCache(eventInfo *C.DD_ETW_EVENT_INFO) {

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPCacheTraceTaskFlushedCache event (id:%v, seq:%v)\n", eventInfo.event.id, eventCount)
	}

	// typedef struct _EVENT_PARAM_HttpService_HTTPCacheTraceTaskAddedCacheEntry
	// {
	//      	uint8_t  uri[1];         // Unicode wide char zero terminating string
	//      //  uint16_t statusCode;
	//      //  uint8_t  verb[1];        // ASCII wide char zero terminating string
	//      //  uint32_t headerLength;
	//      //  uint32_t contentLength;
	//      //  uint64_t expirationTime;
	// } EVENT_PARAM_HttpService_HTTPCacheTraceTaskAddedCacheEntry;

	userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

	// Parse Url
	urlOffset := 0
	url, _, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		fmt.Printf("*** Error: HTTPCacheTraceTaskFlushedCache could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n", urlTermZeroIdx)
		return
	}

	if OutputLevel == OutputVeryVerbose {
		httpReqCacheEntry, found := sysCache[url]
		if !found {
			missedCacheCount++
			fmt.Printf("Warning: HTTPCacheTraceTaskFlushedCache failed to find cached url %v\n\n", url)
			return
		}

		fmt.Printf("  Url:            %v\n", url)
		fmt.Printf("  StatusCode:     %v\n", httpReqCacheEntry.statusCode)
		fmt.Printf("  Verb:           %v\n", httpReqCacheEntry.verb)
		fmt.Printf("  HeaderLength:   %v\n", httpReqCacheEntry.headerLength)
		fmt.Printf("  ContentLength:  %v\n", httpReqCacheEntry.contentLength)

		if httpReqCacheEntry.reqRespBound {
			fmt.Printf("  SiteID:         %v\n", httpReqCacheEntry.http.siteID)
			fmt.Printf("  AppPool:        %v\n", httpReqCacheEntry.http.appPool)
		}

		fmt.Printf("\n")
	}

	// Delete it from sysCache
	delete(sysCache, url)
}

// -----------------------------------------------------------
// HttpService ETW Event #10-14 (HTTPRequestTraceTaskXXXSendXXX)
//
func httpCallbackOnHTTPRequestTraceTaskSend(eventInfo *C.DD_ETW_EVENT_INFO) {

	// We probably should use this even as a last event for a particular activity and use
	// it to better measure duration is http procesing

	var eventName string
	if OutputLevel == OutputVeryVerbose {
		switch eventInfo.event.id {
		case C.EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete:
			eventName = "HTTPRequestTraceTaskSendComplete"
		case C.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend:
			eventName = "HTTPRequestTraceTaskCachedAndSend"
		case C.EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend:
			eventName = "HTTPRequestTraceTaskFastSend"
		case C.EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend:
			eventName = "HTTPRequestTraceTaskZeroSend"
		case C.EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError:
			eventName = "HTTPRequestTraceTaskLastSndError"
		default:
			eventName = "Unknown Send"
		}

		fmt.Printf("Http-service: %v event (id:%v, seq:%v)\n", eventName, eventInfo.event.id, eventCount)
	}

	// Get req/resp conn link
	reqRespAndLink, found := reqResp2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. %v failed to find connection ActivityID\n\n",
				eventName, formatGuid(eventInfo.event.activityId))
		}
		return
	}

	completeReqRespTracking(eventInfo, reqRespAndLink)
}

func httpCallbackOnHttpServiceNonProcessedEvents(eventInfo *C.DD_ETW_EVENT_INFO) {
	notHandledEventsCount++

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: Other non-processed events (id:%v, seq:%v)\n\n", eventInfo.event.id, eventCount)
	}
}

func etwHttpServiceSummary() {
	lastSummaryTime = time.Now()
	summaryCount++

	fmt.Printf("=====================\n")
	fmt.Printf("  SUMMARY #%v\n", summaryCount)
	fmt.Printf("=====================\n")
	fmt.Printf("  Pid:                     %v\n", os.Getpid())
	fmt.Printf("  Conn map:                %v\n", len(connOpened))
	fmt.Printf("  Req/Resp map:            %v\n", len(reqResp2openConn))
	fmt.Printf("  Cache map:               %v\n", len(sysCache))
	fmt.Printf("  All Events(not handled): %v(%v)\n", formatUInt(eventCount), formatUInt(notHandledEventsCount))
	fmt.Printf("  Requests(cached):        %v(%v)\n", formatUInt(completedRequestCount), formatUInt(servedFromCache))
	fmt.Printf("  Missed Conn:             %v\n", formatUInt(missedConnectionCount))
	fmt.Printf("  Parsing Error:           %v\n", formatUInt(parsingErrorCount))
	fmt.Printf("  ETW Total DataL:         %v\n", bytesFormat(transferedETWDataLength))

	if curProc, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if cpu, err2 := curProc.CPUPercent(); err2 == nil {
			fmt.Printf("  CPU:                     %.2f%%\n", cpu)
		}

		if memInfo, err2 := curProc.MemoryInfo(); err2 == nil {
			fmt.Printf("  VMS(RSS):                %v(%v)\n", bytesFormat(memInfo.VMS), bytesFormat(memInfo.RSS))
		}
	}

	fmt.Print("\n")
}

func etwHttpServiceCallback(eventInfo *C.DD_ETW_EVENT_INFO) {

	// output summary every 20 seconds
	if OutputLevel != OutputNone {
		if time.Since(lastSummaryTime).Seconds() >= 20 {
			etwHttpServiceSummary()
		}
	}

	transferedETWDataLength += uint64(eventInfo.event.userDataLength)

	eventCount++

	switch eventInfo.event.id {
	// #21
	case C.EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn:
		httpCallbackOnHTTPConnectionTraceTaskConnConn(eventInfo)

	// #23
	case C.EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose:
		httpCallbackOnHTTPConnectionTraceTaskConnClose(eventInfo)

	// #1
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq:
		httpCallbackOnHTTPRequestTraceTaskRecvReq(eventInfo)

	// #2
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskParse:
		httpCallbackOnHTTPRequestTraceTaskParse(eventInfo)

	// #3
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver:
		httpCallbackOnHTTPRequestTraceTaskDeliver(eventInfo)

	// #4, #8
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp:
		fallthrough
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp:
		httpCallbackOnHTTPRequestTraceTaskRecvResp(eventInfo)

	// #16, #17
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache:
		fallthrough
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified:
		httpCallbackOnHTTPRequestTraceTaskSrvdFrmCache(eventInfo)

	// #25
	case C.EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry:
		httpCallbackOnHTTPCacheTraceTaskAddedCacheEntry(eventInfo)

	// #27
	case C.EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache:
		httpCallbackOnHTTPCacheTraceTaskFlushedCache(eventInfo)

	// #10-14
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete:
		fallthrough
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend:
		fallthrough
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend:
		fallthrough
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend:
		fallthrough
	case C.EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError:
		httpCallbackOnHTTPRequestTraceTaskSend(eventInfo)

	default:
		httpCallbackOnHttpServiceNonProcessedEvents(eventInfo)
	}

	// Simulate collection of closed connection
	// keep slice "cap" allocated
	if len(connClosed) > 500 {
		connClosed = connClosed[:0]
	}
}
