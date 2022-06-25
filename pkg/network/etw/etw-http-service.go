// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//	/////////////////////////////////////////////////////////////////////////////////////////
//	Before understand the flow and the code I recommend to install Windows SDK with Performance
//	Analisis enabled. Experiment using following approach
//
//    1. Capture under different HTTP load and profile scenario and save it to a file (http.etl)
//	   a.xperf -on PROC_THREAD+LOADER+Base -start httptrace -on Microsoft-Windows-HttpService
//       b.  ... initiate http connections using various profiles
//       c. xperf -stop -stop httptrace -d http.etl
//
//	2. Load into Windows Performance Analyzer by double click on http.etl file
//
//	3. Display Event window and filter only to Microsoft-Windows-HttpService events
//	  a. Double click on "System-Activity/Generic Events" on a left to open Generic Events
//	     Windows.
//	  b. Select Microsoft-Windows-HttpService in the Series windows, right mouse button click
//	     on it and select the "Filter to selection" menu item.
//
//	4. Sort HTTP events in time ascending order and make few other column choices to maximize
//	   the screen
//	  a. Right button click in the column bar and select the "Open View Editor ..." menu
//	  b. Drag "DateTime(local)" before "Task Name"
//	  c. Drag "etw:ActivityId" after "DateTime" name
//	  d. Drag "etw:Related ActivityId" after "etw:ActivityId" name
//	  e. Uncheck "Provider Name"
//	  f. Uncheck "Event Name"
//	  g. Uncheck "cpu"
//
//	/////////////////////////////////////////////////////////////////////////////////////////
//    HTTP and App Pool info detection performance overhead
//
//	To detect HTTP and App Pool information I had to activate Microsoft-Windows-HttpService
//	ETW source and from "atomic" ETW events create synthetic HTTP events. It seems to be
//	 working well but its performance impact is not negligent.
//
//	Roughly speaking, in terms of overhead, there are 3 distinct activities used to generate
//	a HTTP event. Here they are with their respective overhead:
//
//	   * [~45% of total overhead] ETW Data Transfer from Kernel.
//	       Windows implicitly transfers ETW event data blobs about HTTP activity from kernel
//		   to our go process pace and invoking our ETW event handler callback.
//
//       * [~35% of total overhead] ETW Data Parsing.
//	       Our Callback is parsing HTTP strings, numbers and TCPIP structs from the passed
//		   from kernel ETW event data blobs.
//
//	   * [~20% of total overhead] Parsed Data Storage and Correlation.
//	       Parsed data needs to be stored in few maps and correlated to eventually
//		   "manufacture" a complete HTTP event (and store it to for the final consumption).
//
//	On a 16 CPU machine collecting 3k per second HTTP events via Microsoft-Windows-HttpService
//	ETW source costs 0.7%-1% of CPU usage.
//
//   On a 16 CPU machine collecting 15k per second HTTP events via Microsoft-Windows-HttpService
//   ETW source costs 4-5% of CPU usage.  During 5 minutes of sustained 15k per second HTTP request
//   loads:
//      * 9,000,000 HTTP requests had been processed
//      * 36,000,000 ETW events had been reported (9,000,000 events were not "interesting" and
//	    were not processed).
//      * 2.4 Gb of data transferred to user mode and had to be parsed and correlated.
//
//    Most likely the cost of HTTP and App Pool detection will be slightly higher after I integrate
//	it into system-probe due to additional correlation or correlations. In addition I did not
//	count CPU cost at the source (HTTP.sys driver) and ETW infrastructure (outside of 45% of overhead)
//	which certainly exists but I am not sure how to measure that. On the other hand I have been
//	trying to code in an efficient manner and perhaps there is room for further optimization (although
//	almost half of the overhead cannot be optimized).
//
//	/////////////////////////////////////////////////////////////////////////////////////////
//	Flows
//
//	1. HTTP transactions events are always in the scope of
//		HTTPConnectionTraceTaskConnConn   21 [Local & Remote IP/Ports]
//		HTTPConnectionTraceTaskConnClose  23
//
//
//	2. HTTP Req/Resp (the same ActivityID)
//	   a. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
//		  HTTPRequestTraceTaskFastResp       8     [statusCode, verb, headerLen, cachePolicy]
//		  HTTPRequestTraceTaskFastSend      12     [httpStatus]
//
//		  or
//
//	   b. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
//		  HTTPRequestTraceTaskFastResp       4     [statusCode, verb, headerLen, cachePolicy = 0]
//		  HTTPRequestTraceTaskSendComplete  10     [httpStatus]
//
//		  or
//
//	   c. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
//		  HTTPRequestTraceTaskFastResp       4     [statusCode, verb, headerLen, cachePolicy=1]
//		  HTTPRequestTraceTaskSrvdFrmCache  16     [site, bytesSent]
//		  HTTPRequestTraceTaskCachedAndSend 11     [httpStatus]
//
//		  or
//
//	   d. HTTPRequestTraceTaskRecvReq        1     [Correlated to Conncetion by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//		  HTTPRequestTraceTaskSrvdFrmCache  16     [site, bytesSent]
//
//	3. HTTP Cache
//	    HTTPCacheTraceTaskAddedCacheEntry   25     [uri, statusCode, verb, headerLength, contentLength] [Correlated to http req/resp by url]
//		HTTPCacheTraceTaskFlushedCache      27     [uri, statusCode, verb, headerLength, contentLength]
//

//go:build windows && npm
// +build windows,npm

package etw

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/shirou/gopsutil/process"
)

// 	"github.com/DataDog/datadog-agent/pkg/network/driver"

/*
#include "etw.h"
#include "etw-provider.h"
*/
import "C"

type HttpTxAux struct {
	Url           string
	AppPool       string
	SiteID        uint32
	HeaderLength  uint32
	ContentLength uint32
}

type Http struct {
	httpTx    driver.HttpTransactionType
	httpTxAux HttpTxAux
	fromCache bool
}

type Conn struct {
	Tup          driver.ConnTupleType
	connected    uint64
	disconnected uint64
}

type ConnOpen struct {
	// conntuple
	conn Conn

	// http link
	httpPendingBackLinks map[C.DDGUID]struct{}
}

type HttpConnLink struct {
	connActivityId C.DDGUID

	http Http
}

type Cache struct {
	statusCode     uint16
	verb           string
	headerLength   uint32
	contentLength  uint32
	expirationTime uint64
	reqRespBound   bool
}

type HttpCache struct {
	cache Cache
	http  Http
}

type ConnHttp struct {
	// conntuple
	Conn Conn

	// http
	Http Http
}

const (
	OutputNone int = iota
	OutpuSummary
	OutputVerbose
	OutputVeryVerbose
)

var (
	OutputLevel int = OutputNone
)

var (
	connOpened    map[C.DDGUID]*ConnOpen
	http2openConn map[C.DDGUID]*HttpConnLink
	httpCache     map[string]*HttpCache

	completedHttpTx            []driver.HttpTransactionType
	completedHttpTxAux         []HttpTxAux
	completedConnHttpMux       sync.Mutex
	completedConnHttpMaxcCount uint64 = 0

	summaryCount            uint64
	eventCount              uint64
	servedFromCache         uint64
	completedRequestCount   uint64
	missedConnectionCount   uint64
	missedCacheCount        uint64
	parsingErrorCount       uint64
	notHandledEventsCount   uint64
	transferedETWDataLength uint64

	lastSummaryTime time.Time
)

func init() {
	initializeEtwHttpServiceSubscription()
}

func completeReqRespTracking(eventInfo *C.DD_ETW_EVENT_INFO, httpConnLink *HttpConnLink) {

	// Get connection
	connOpen, connFound := connOpened[httpConnLink.connActivityId]
	if !connFound {
		missedConnectionCount++

		// No connection, no potint to keep it longer inthe pending HttpReqRespMap
		delete(http2openConn, eventInfo.event.activityId)

		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskFastResp failed to find connection object\n\n",
				formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId))
		}
		return
	}

	// Time
	httpConnLink.http.httpTx.ResponseLastSeen = fileTimeToUnixTime(uint64(eventInfo.event.timeStamp))

	// Clean it up related containers
	delete(http2openConn, eventInfo.event.activityId)
	delete(connOpen.httpPendingBackLinks, eventInfo.event.activityId)

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  CompletedReq:   %v\n", completedRequestCount)
		fmt.Printf("  Connected:      %v\n", connOpen.conn.connected)
		fmt.Printf("  Requested:      %v\n", formatUnixTime(httpConnLink.http.httpTx.RequestStarted))
		fmt.Printf("  Responded:      %v\n", formatUnixTime(httpConnLink.http.httpTx.ResponseLastSeen))
		fmt.Printf("  ConnActivityId: %v\n", formatGuid(httpConnLink.connActivityId))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		if connFound {
			fmt.Printf("  Local:          %v:%v\n", ip4format(connOpen.conn.Tup.SrvAddr), connOpen.conn.Tup.SrvPort)
			fmt.Printf("  Remote:         %v:%v\n", ip4format(connOpen.conn.Tup.CliAddr), connOpen.conn.Tup.CliPort)
		}
		fmt.Printf("  Cached:         %v\n", httpConnLink.http.fromCache)
		fmt.Printf("  AppPool:        %v\n", httpConnLink.http.httpTxAux.AppPool)
		fmt.Printf("  Url:            %v\n", httpConnLink.http.httpTxAux.Url)
		fmt.Printf("  Method:         %v\n", httpMethodToStr(httpConnLink.http.httpTx.RequestMethod))
		fmt.Printf("  StatusCode:     %v\n", httpConnLink.http.httpTx.ResponseStatusCode)
		fmt.Printf("  HeaderLength:   %v\n", httpConnLink.http.httpTxAux.HeaderLength)
		fmt.Printf("  ContentLength:  %v\n", httpConnLink.http.httpTxAux.ContentLength)
		fmt.Printf("\n")
	} else if OutputLevel == OutputVerbose {
		fmt.Printf("%v. %v L[%v:%v], R[%v:%v], P[%v], C[%v], V[%v], H[%v], B[%v], U[%v]\n",
			completedRequestCount,
			formatUnixTime(httpConnLink.http.httpTx.RequestStarted),
			ip4format(connOpen.conn.Tup.SrvAddr), connOpen.conn.Tup.SrvPort,
			ip4format(connOpen.conn.Tup.CliAddr), connOpen.conn.Tup.CliPort,
			httpConnLink.http.httpTxAux.AppPool,
			httpConnLink.http.httpTx.ResponseStatusCode,
			httpMethodToStr(httpConnLink.http.httpTx.RequestMethod),
			httpConnLink.http.httpTxAux.HeaderLength,
			httpConnLink.http.httpTxAux.ContentLength,
			httpConnLink.http.httpTxAux.Url)
	}

	completedRequestCount++

	// Http is completed, move it to completed list ...
	completedConnHttpMux.Lock()
	defer completedConnHttpMux.Unlock()
	completedHttpTx = append(completedHttpTx, httpConnLink.http.httpTx)
	completedHttpTxAux = append(completedHttpTxAux, httpConnLink.http.httpTxAux)
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

	var connOpen ConnOpen

	// Local and remote ipaddress and port
	// Currently it is only ip4 support
	connOpen.conn.Tup.SrvPort = binary.BigEndian.Uint16(userData[14:16])
	copy(connOpen.conn.Tup.SrvAddr[:], userData[16:20])
	connOpen.conn.Tup.CliPort = binary.BigEndian.Uint16(userData[34:36])
	copy(connOpen.conn.Tup.CliAddr[:], userData[36:40])

	// Time
	connOpen.conn.connected = fileTimeToUnixTime(uint64(eventInfo.event.timeStamp))

	// Http back links (to cleanup on closure)
	connOpen.httpPendingBackLinks = make(map[C.DDGUID]struct{}, 10)

	// Save to the map
	connOpened[eventInfo.event.activityId] = &connOpen

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Time:           %v\n", formatUnixTime(connOpen.conn.connected))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  Local:          %v:%v\n", ip4format(connOpen.conn.Tup.SrvAddr), connOpen.conn.Tup.SrvPort)
		fmt.Printf("  Remote:         %v:%v\n", ip4format(connOpen.conn.Tup.CliAddr), connOpen.conn.Tup.CliPort)
		fmt.Printf("\n")
	}
}

// -------------------------------------------------------------
// HttpService ETW Event #23 (HTTPConnectionTraceTaskConnClose)
//
func httpCallbackOnHTTPConnectionTraceTaskConnClose(eventInfo *C.DD_ETW_EVENT_INFO) {
	// output details
	connOpen, found := connOpened[eventInfo.event.activityId]
	if found {
		// ... and clean it up related containers
		delete(http2openConn, eventInfo.event.activityId)

		completedRequestCount++

		// move it to close connection
		connOpen.conn.disconnected = fileTimeToUnixTime(uint64(eventInfo.event.timeStamp))

		// Clean pending http2openConn
		for httpReqRespActivityId := range connOpen.httpPendingBackLinks {
			delete(http2openConn, httpReqRespActivityId)
		}

		// ... and remoe itself from the map
		delete(connOpened, eventInfo.event.activityId)

	} else {
		missedConnectionCount++
	}

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("Http-service: HTTPConnectionTraceTaskConnClose event (id:23, seq:%v)\n", eventCount)
		if found {
			fmt.Printf("  ActivityId: %v, Local[%v:%v], Remote[%v:%v])\n",
				formatGuid(eventInfo.event.activityId),
				ip4format(connOpen.conn.Tup.SrvAddr), connOpen.conn.Tup.SrvPort,
				ip4format(connOpen.conn.Tup.CliAddr), connOpen.conn.Tup.CliPort)
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
	// userData := goBytes(unsafe.Pointer(eventInfo.event.userData), C.int(eventInfo.event.userDataLength))

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

	connOpen, connFound := connOpened[eventInfo.event.activityId]
	if !connFound {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. Releated ActivityId:%v. HTTPRequestTraceTaskRecvReq failed to find connection object\n",
				formatGuid(eventInfo.event.activityId), formatGuid(*eventInfo.relatedActivityId))
		}
		return
	}

	// Initialize ReqResp and Conn Link
	reqRespAndLink := &HttpConnLink{}
	reqRespAndLink.connActivityId = eventInfo.event.activityId
	reqRespAndLink.http.httpTx.Tup = connOpen.conn.Tup
	reqRespAndLink.http.httpTx.RequestStarted = fileTimeToUnixTime(uint64(eventInfo.event.timeStamp))

	// Save Req/Resp Conn Link and back reference to it
	http2openConn[*eventInfo.relatedActivityId] = reqRespAndLink
	var dummy struct{}
	connOpen.httpPendingBackLinks[*eventInfo.relatedActivityId] = dummy

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Time:           %v\n", formatUnixTime(reqRespAndLink.http.httpTx.RequestStarted))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  RelActivityId:  %v\n", formatGuid(*eventInfo.relatedActivityId))
		if connFound {
			fmt.Printf("  Local:          %v:%v\n", ip4format(connOpen.conn.Tup.SrvAddr), connOpen.conn.Tup.SrvPort)
			fmt.Printf("  Remote:         %v:%v\n", ip4format(connOpen.conn.Tup.CliAddr), connOpen.conn.Tup.CliPort)
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
	httpConnLink, found := http2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskParse failed to find connection ActivityID\n\n", formatGuid(eventInfo.event.activityId))
		return
	}

	// Verb (in future we can cast number to)
	httpConnLink.http.httpTx.RequestMethod = binary.LittleEndian.Uint32(userData[8:12])

	// Parse Url
	urlOffset := 12
	url, _, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. HTTPRequestTraceTaskParse could not find terminating zero for Url. termZeroIdx=%v\n\n",
			formatGuid(eventInfo.event.activityId), urlTermZeroIdx)

		// If problem stop tracking this
		delete(http2openConn, eventInfo.event.activityId)
		return
	}

	httpConnLink.http.httpTxAux.Url = url

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  Url:            %v\n", httpConnLink.http.httpTxAux.Url)
		fmt.Printf("  Method:         %v\n", httpMethodToStr(httpConnLink.http.httpTx.RequestMethod))
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
	httpConnLink, found := http2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskDeliver failed to find connection ActivityID\n\n", formatGuid(eventInfo.event.activityId))
		}
		return
	}

	// Extra output
	connOpen, connFound := connOpened[httpConnLink.connActivityId]
	if !connFound {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver failed to find connection object\n",
				formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId))
		}

		// If no connection found then stop tracking
		delete(http2openConn, eventInfo.event.activityId)
		return
	}

	// Parse RequestQueueName
	appPoolOffset := 20
	appPool, urlOffset, appPoolFound, appPoolTermZeroIdx := parseUnicodeString(userData, appPoolOffset)
	if !appPoolFound {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId), appPoolTermZeroIdx)

		// If problem stop tracking this
		delete(http2openConn, eventInfo.event.activityId)
		return
	}

	httpConnLink.http.httpTxAux.AppPool = appPool

	// Parse url
	if urlOffset > len(userData) {
		parsingErrorCount++

		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find begining of Url\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId))

		// If problem stop tracking this
		delete(http2openConn, eventInfo.event.activityId)
		return
	}

	// Parse url (skip it because it is already captured httpCallbackOnHTTPRequestTraceTaskParse already)
	// Previous implementation (we can use it in future if configured to cross-validation)
	//    url, _, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	//    reqRespAndLink.http.url = url
	//    if !urlFound {
	//    	parsingErrorCount++
	//    	fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find terminating zero for url. termZeroIdx=%v\n\n",
	//    		formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId), urlTermZeroIdx)
	//
	//    	// If problem stop tracking this
	//    	delete(reqResp2openConn, eventInfo.event.activityId)
	//    	return
	//    }
	//    reqRespAndLink.http.Url = url

	// output details
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  ConnActivityId: %v\n", formatGuid(httpConnLink.connActivityId))
		fmt.Printf("  ActivityId:     %v\n", formatGuid(eventInfo.event.activityId))
		fmt.Printf("  AppPool:        %v\n", httpConnLink.http.httpTxAux.AppPool)
		fmt.Printf("  Url:            %v\n", httpConnLink.http.httpTxAux.Url)
		if connFound {
			fmt.Printf("  Local:          %v:%v\n", ip4format(connOpen.conn.Tup.SrvAddr), connOpen.conn.Tup.SrvPort)
			fmt.Printf("  Remote:         %v:%v\n", ip4format(connOpen.conn.Tup.CliAddr), connOpen.conn.Tup.CliPort)
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
	httpConnLink, found := http2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskXxxResp failed to find connection ActivityID\n\n",
				formatGuid(eventInfo.event.activityId))
		}
		return
	}
	httpConnLink.http.httpTx.ResponseStatusCode = binary.LittleEndian.Uint16(userData[16:18])

	// Parse Verb (just skip it, we already get verb string value from int in httpCallbackOnHTTPRequestTraceTaskParse)
	// Previous implementation (Previous implementation (we can use it in future if configured to cross-validation)
	//     verb, headerSizeOffset, verbFound, verbTermZeroIdx := parseAsciiString(userData, verbOffset)
	//     reqRespAndLink.http.verb = verb
	verbOffset := 18
	headerSizeOffset, verbFound, verbTermZeroIdx := skipAsciiString(userData, verbOffset)
	if !verbFound {
		parsingErrorCount++
		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskXxxResp could not find terminating zero for Verb. termZeroIdx=%v\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId), verbTermZeroIdx)
		return
	}

	// Parse headerLength (space for 32bit number)
	if (headerSizeOffset + 4) > len(userData) {
		fmt.Printf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskXxxResp Not enough space for HeaderLength. userDataSize=%v, parsedDataSize=%v\n\n",
			formatGuid(eventInfo.event.activityId), formatGuid(httpConnLink.connActivityId), len(userData), (headerSizeOffset + 4))
		return
	}

	httpConnLink.http.httpTxAux.HeaderLength = binary.LittleEndian.Uint32(userData[headerSizeOffset:])

	httpConnLink.http.fromCache = false
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
	httpConnLink, found := http2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. HTTPRequestTraceTaskSrvdFrmCache failed to find connection ActivityID\n\n",
				formatGuid(eventInfo.event.activityId))
		}
		return
	}

	// Get from cache or set to the sysCache
	cacheEntry, found := httpCache[httpConnLink.http.httpTxAux.Url]
	if !found {
		fmt.Printf("Warning: HTTPRequestTraceTaskSrvdFrmCache failed to find HTTP cache entry by url %v\n\n", httpConnLink.http.httpTxAux.Url)

		// If problem stop tracking this
		if OutputLevel == OutputVeryVerbose {
			delete(http2openConn, eventInfo.event.activityId)
		}
		return
	}

	// Log the findings
	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Cache entry for %v is found\n", httpConnLink.http.httpTxAux.Url)
		if cacheEntry.cache.reqRespBound {
			fmt.Printf("  Completing reqResp tracking\n")
		} else {
			fmt.Printf("  Updating cache entry via current http request\n")
		}
		fmt.Printf("\n")
	}

	if cacheEntry.cache.reqRespBound {
		// Get from cache and complete reqResp tracking
		httpConnLink.http = cacheEntry.http
		httpConnLink.http.fromCache = true
		httpConnLink.http.httpTxAux.AppPool = cacheEntry.http.httpTxAux.AppPool
		httpConnLink.http.httpTxAux.SiteID = cacheEntry.http.httpTxAux.SiteID
		httpConnLink.http.httpTx.ResponseStatusCode = cacheEntry.http.httpTx.ResponseStatusCode

		completeReqRespTracking(eventInfo, httpConnLink)
		servedFromCache++
	} else {
		// Set to cache
		cacheEntry.cache.reqRespBound = true
		cacheEntry.http = httpConnLink.http
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

	cacheEntry := &HttpCache{}

	// Parse Url
	urlOffset := 0
	url, statusCodeOffset, urlFound, urlTermZeroIdx := parseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n", urlTermZeroIdx)
		return
	}

	// Status code
	cacheEntry.cache.statusCode = binary.LittleEndian.Uint16(userData[statusCodeOffset : statusCodeOffset+2])

	// Parse Verb
	verbOffset := statusCodeOffset + 2
	verb, headerSizeOffset, verbFound, verbTermZeroIdx := parseAsciiString(userData, verbOffset)
	if !verbFound {
		parsingErrorCount++
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry could not find terminating zero for Verb. termZeroIdx=%v\n\n", verbTermZeroIdx)
		return
	}
	cacheEntry.cache.verb = verb

	// Parse headerLength (space for 32bit number)
	if (headerSizeOffset + 4) > len(userData) {
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry Not enough space for HeaderLength. userDataSize=%v, parsedDataSize=%v\n\n",
			len(userData), (headerSizeOffset + 4))
		return
	}
	cacheEntry.cache.headerLength = binary.LittleEndian.Uint32(userData[headerSizeOffset:])

	// Parse contentLength (space for 32bit number)
	contentLengthOffset := headerSizeOffset + 4
	if (contentLengthOffset + 4) > len(userData) {
		fmt.Printf("*** Error: HTTPCacheTraceTaskAddedCacheEntry Not enough space for contentLengthOffset. userDataSize=%v, parsedDataSize=%v\n\n",
			len(userData), (contentLengthOffset + 4))
		return
	}
	cacheEntry.cache.contentLength = binary.LittleEndian.Uint32(userData[contentLengthOffset:])

	cacheEntry.cache.reqRespBound = false

	// Save it to sysCache
	httpCache[url] = cacheEntry

	if OutputLevel == OutputVeryVerbose {
		fmt.Printf("  Url:            %v\n", url)
		fmt.Printf("  StatusCode:     %v\n", cacheEntry.cache.statusCode)
		fmt.Printf("  Verb:           %v\n", cacheEntry.cache.verb)
		fmt.Printf("  HeaderLength:   %v\n", cacheEntry.cache.headerLength)
		fmt.Printf("  ContentLength:  %v\n", cacheEntry.cache.contentLength)
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
		cacheEntry, found := httpCache[url]
		if !found {
			missedCacheCount++
			fmt.Printf("Warning: HTTPCacheTraceTaskFlushedCache failed to find cached url %v\n\n", url)
			return
		}

		fmt.Printf("  Url:            %v\n", url)
		fmt.Printf("  StatusCode:     %v\n", cacheEntry.cache.statusCode)
		fmt.Printf("  Verb:           %v\n", cacheEntry.cache.verb)
		fmt.Printf("  HeaderLength:   %v\n", cacheEntry.cache.headerLength)
		fmt.Printf("  ContentLength:  %v\n", cacheEntry.cache.contentLength)

		if cacheEntry.cache.reqRespBound {
			fmt.Printf("  SiteID:         %v\n", cacheEntry.http.httpTxAux.SiteID)
			fmt.Printf("  AppPool:        %v\n", cacheEntry.http.httpTxAux.AppPool)
		}

		fmt.Printf("\n")
	}

	// Delete it from sysCache
	delete(httpCache, url)
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
	httpConnLink, found := http2openConn[eventInfo.event.activityId]
	if !found {
		missedConnectionCount++
		if OutputLevel == OutputVeryVerbose {
			fmt.Printf("Warning: ActivityId:%v. %v failed to find connection ActivityID\n\n",
				eventName, formatGuid(eventInfo.event.activityId))
		}
		return
	}

	completeReqRespTracking(eventInfo, httpConnLink)
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
	fmt.Printf("  Req/Resp map:            %v\n", len(http2openConn))
	fmt.Printf("  Cache map:               %v\n", len(httpCache))
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
}

// can be called multiple times
func initializeEtwHttpServiceSubscription() {
	connOpened = make(map[C.DDGUID]*ConnOpen)
	http2openConn = make(map[C.DDGUID]*HttpConnLink)
	httpCache = make(map[string]*HttpCache, 100)

	connOpened = make(map[C.DDGUID]*ConnOpen)
	http2openConn = make(map[C.DDGUID]*HttpConnLink)
	httpCache = make(map[string]*HttpCache, 100)

	summaryCount = 0
	eventCount = 0
	servedFromCache = 0
	completedRequestCount = 0
	missedConnectionCount = 0
	missedCacheCount = 0
	parsingErrorCount = 0
	notHandledEventsCount = 0
	transferedETWDataLength = 0

	lastSummaryTime = time.Now()

	if completedConnHttpMaxcCount == 0 {
		completedConnHttpMaxcCount = 10000
	}

	completedConnHttpMux.Lock()
	defer completedConnHttpMux.Unlock()
	completedHttpTx = make([]driver.HttpTransactionType, 0, 100)
	completedHttpTxAux = make([]HttpTxAux, 0, 100)
}

func ReadConnHttp() ([]driver.HttpTransactionType, []HttpTxAux) {
	completedConnHttpMux.Lock()
	defer completedConnHttpMux.Unlock()

	// Return accumulated ConnHttp array and reset array
	readHttpTx := completedHttpTx
	completedHttpTx = make([]driver.HttpTransactionType, 0, 100)
	readHttpTxAux := completedHttpTxAux
	completedHttpTxAux = make([]HttpTxAux, 0, 100)

	return readHttpTx, readHttpTxAux
}

func SetMaxFlows(maxFlows uint64) {
	completedConnHttpMaxcCount = maxFlows
}

func StopEtwHttpServiceSubscription() {
	initializeEtwHttpServiceSubscription()
}
