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
//  On a 16 CPU machine collecting 15k per second HTTP events via Microsoft-Windows-HttpService
//  ETW source costs 4-5% of CPU usage.  During 5 minutes of sustained 15k per second HTTP request
//  loads:
//      * 9,000,000 HTTP requests had been processed
//      * 36,000,000 ETW events had been reported (9,000,000 events were not "interesting" and
//	    were not processed).
//      * 6 Gb of data transferred to user mode and some of that had to be parsed and correlated.
//        Header comprised from 3.6 Gb (112 bytes per event) and payload 2.4 Gb
//
//  Note:
//      Theses are early measurements and need to be reevaluted because filtering since improved
//      and less events should be pushed through and reduce the overhead
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
//	   a. HTTPRequestTraceTaskRecvReq        1     [Correlated to Connection by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
//		  HTTPRequestTraceTaskFastResp       8     [statusCode, verb, headerLen, cachePolicy]
//		  HTTPRequestTraceTaskFastSend      12     [httpStatus]
//
//		  or
//
//	   b. HTTPRequestTraceTaskRecvReq        1     [Correlated to Connection by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
//		  HTTPRequestTraceTaskFastResp       4     [statusCode, verb, headerLen, cachePolicy = 0]
//		  HTTPRequestTraceTaskSendComplete  10     [httpStatus]
//
//		  or
//
//	   c. HTTPRequestTraceTaskRecvReq        1     [Correlated to Connection by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//	      HTTPRequestTraceTaskDeliver        3     [siteId, reqQueueName, url]
//		  HTTPRequestTraceTaskFastResp       4     [statusCode, verb, headerLen, cachePolicy=1]
//		  HTTPRequestTraceTaskSrvdFrmCache  16     [site, bytesSent]
//		  HTTPRequestTraceTaskCachedAndSend 11     [httpStatus]
//
//		  or
//
//	   d. HTTPRequestTraceTaskRecvReq        1     [Correlated to Connection by builtin ActivityID<->ReleatedActivityID]
//	      HTTPRequestTraceTaskParse          2     [verb, url]
//		  HTTPRequestTraceTaskSrvdFrmCache  16     [site, bytesSent]
//
//	3. HTTP Cache
//	    HTTPCacheTraceTaskAddedCacheEntry   25     [uri, statusCode, verb, headerLength, contentLength] [Correlated to http req/resp by url]
//		HTTPCacheTraceTaskFlushedCache      27     [uri, statusCode, verb, headerLength, contentLength]
//

//go:build windows && npm

package http

import (
	"encoding/binary"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
)

type Http struct {
	// Most of it just like driver's HTTP data ...
	Txn driver.HttpTransactionType
	// To keep up with WinHttpTransaction (cannot import http package - will be cyclical import)
	// Probably need to move to common windows utility package
	RequestFragment []byte

	// ... plus some extra
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

type Conn struct {
	tup          driver.ConnTupleType
	connected    uint64
	disconnected uint64
}

type ConnOpen struct {
	// conntuple
	conn Conn

	// SSL (tracked only when HttpServiceLogVerbosity == HttpServiceLogVeryVerbose)
	// by default Go object will have it false which works for us
	ssl bool

	// http link
	httpPendingBackLinks map[etw.DDGUID]struct{}
}

type HttpConnLink struct {
	connActivityId etw.DDGUID

	http WinHttpTransaction

	url string
}

type Cache struct {
	statusCode uint16
	// <<<MORE ETW HttpService DETAILS>>>
	// verb           string
	// headerLength   uint32
	// contentLength  uint32
	// expirationTime uint64
	reqRespBound bool
}

type HttpCache struct {
	cache Cache
	http  WinHttpTransaction
}

const (
	HttpServiceLogNone int = iota
	HttpServiceLogSummary
	HttpServiceLogVerbose
	HttpServiceLogVeryVerbose
)

var (
	// Should be controlled by config
	HttpServiceLogVerbosity int = HttpServiceLogSummary
)

var (
	httpServiceSubscribed bool = false
	connOpened            map[etw.DDGUID]*ConnOpen
	http2openConn         map[etw.DDGUID]*HttpConnLink
	httpCache             map[string]*HttpCache

	completedHttpTxMux      sync.Mutex
	completedHttpTx         []WinHttpTransaction
	completedHttpTxMaxCount uint64 = 1000 // default max
	maxRequestFragmentBytes uint64 = 25
	completedHttpTxDropped  uint   = 0 // when should we reset this telemetry and how to expose it

	captureHTTP  bool
	captureHTTPS bool

	summaryCount              uint64
	eventCount                uint64
	servedFromCache           uint64
	completedRequestCount     uint64
	missedConnectionCount     uint64
	missedCacheCount          uint64 //nolint:unused
	parsingErrorCount         uint64
	notHandledEventsCount     uint64
	transferedETWBytesTotal   uint64
	transferedETWBytesPayload uint64

	lastSummaryTime time.Time

	iisConfig *winutil.DynamicIISConfig
)

func init() {
	initializeEtwHttpServiceSubscription()

}

func cleanupActivityIdViaConnOpen(connOpen *ConnOpen, activityId etw.DDGUID) {
	// Clean it up related containers
	delete(http2openConn, activityId)
	delete(connOpen.httpPendingBackLinks, activityId)
}

func cleanupActivityIdViaConnActivityId(connActivityId etw.DDGUID, activityId etw.DDGUID) {
	connOpen, connFound := connOpened[connActivityId]
	if connFound {
		cleanupActivityIdViaConnOpen(connOpen, activityId)
	}
}

func getConnOpen(activityId etw.DDGUID) (*ConnOpen, bool) {
	connOpen, connFound := connOpened[activityId]
	if !connFound {
		if captureHTTPS || HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
			missedConnectionCount++
			if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
				log.Warnf("* Warning!!!: ActivityId:%v. Failed to find connection object\n\n", etw.FormatGUID(activityId))
			}
		}
		return nil, false
	}

	return connOpen, connFound
}

func getHttpConnLink(activityId etw.DDGUID) (*HttpConnLink, bool) {
	httpConnLink, found := http2openConn[activityId]
	if !found {
		if captureHTTPS || HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
			missedConnectionCount++
			if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
				log.Warnf("* Warning: ActivityId:%v. Failed to find connection ActivityID\n\n", etw.FormatGUID(activityId))
			}
		}

		return nil, false
	}

	return httpConnLink, found
}

func completeReqRespTracking(eventInfo *etw.DDEtwEventInfo, httpConnLink *HttpConnLink) {

	// Get connection
	connOpen, connFound := connOpened[httpConnLink.connActivityId]
	if !connFound {
		missedConnectionCount++

		// No connection, no potint to keep it longer inthe pending HttpReqRespMap
		delete(http2openConn, eventInfo.Event.ActivityId)

		if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
			log.Warnf("* Warning!!!: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskFastResp failed to find connection object\n\n",
				etw.FormatGUID(eventInfo.Event.ActivityId), etw.FormatGUID(httpConnLink.connActivityId))
		}
		return
	}

	// Time
	httpConnLink.http.Txn.ResponseLastSeen = winutil.FileTimeToUnixNano(uint64(eventInfo.Event.TimeStamp))

	// Clean it up related containers
	cleanupActivityIdViaConnOpen(connOpen, eventInfo.Event.ActivityId)

	// output details
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  CompletedReq:   %v\n", completedRequestCount)
		log.Infof("  Connected:      %v\n", connOpen.conn.connected)
		log.Infof("  Requested:      %v\n", etw.FormatUnixTime(httpConnLink.http.Txn.RequestStarted))
		log.Infof("  Responded:      %v\n", etw.FormatUnixTime(httpConnLink.http.Txn.ResponseLastSeen))
		log.Infof("  ConnActivityId: %v\n", etw.FormatGUID(httpConnLink.connActivityId))
		log.Infof("  ActivityId:     %v\n", etw.FormatGUID(eventInfo.Event.ActivityId))
		if connFound {
			log.Infof("  Local:          %v\n", etw.IPFormat(connOpen.conn.tup, true))
			log.Infof("  Remote:         %v\n", etw.IPFormat(connOpen.conn.tup, false))
			log.Infof("  Family:         %v\n", connOpen.conn.tup.Family)
		}
		log.Infof("  AppPool:        %v\n", httpConnLink.http.AppPool)
		log.Infof("  Url:            %v\n", httpConnLink.url)
		log.Infof("  Method:         %v\n", etw.HTTPMethodToStr(httpConnLink.http.Txn.RequestMethod))
		log.Infof("  StatusCode:     %v\n", httpConnLink.http.Txn.ResponseStatusCode)
		// <<<MORE ETW HttpService DETAILS>>>
		// log.Infof("  HeaderLength:   %v\n", httpConnLink.http.HeaderLength)
		// log.Infof("  ContentLength:  %v\n", httpConnLink.http.ContentLength)
		log.Infof("\n")
	} else if HttpServiceLogVerbosity == HttpServiceLogVerbose {
		log.Infof("%v. %v L[%v], R[%v], F[%v], P[%v], C[%v], V[%v], U[%v]\n",
			completedRequestCount,
			etw.FormatUnixTime(httpConnLink.http.Txn.RequestStarted),
			etw.IPFormat(connOpen.conn.tup, true),
			etw.IPFormat(connOpen.conn.tup, false),
			connOpen.conn.tup.Family,
			httpConnLink.http.AppPool,
			httpConnLink.http.Txn.ResponseStatusCode,
			etw.HTTPMethodToStr(httpConnLink.http.Txn.RequestMethod),
			// <<<MORE ETW HttpService DETAILS>>>
			// httpConnLink.http.HeaderLength,
			// httpConnLink.http.ContentLength,
			// httpConnLink.http.url, (url moved to httpConnLink.url)
			httpConnLink.url)
	}

	completedRequestCount++

	if !captureHTTP && !connOpen.ssl {
		if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
			log.Infof("Dropping HTTP connection")
		}
		return
	}
	if !captureHTTPS && connOpen.ssl {
		if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
			log.Infof("Dropping HTTPS connection")
		}
		return
	}

	// Http is completed, move it to completed list ...
	completedHttpTxMux.Lock()
	defer completedHttpTxMux.Unlock()

	if uint64(len(completedHttpTx)) <= completedHttpTxMaxCount {
		completedHttpTx = append(completedHttpTx, httpConnLink.http)
	} else {
		completedHttpTxDropped++
	}
}

// ============================================
//
// E T W    E v e n t s   H a n d l e r s
//

// -----------------------------------------------------------
// HttpService ETW Event #21 (HTTPConnectionTraceTaskConnConn)
func httpCallbackOnHTTPConnectionTraceTaskConnConn(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}

	// -----------------------------------
	// IP4
	//
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

	// -----------------------------------
	// IP6
	//
	// 28 bytes address mapped to sockaddr_in6 (https://docs.microsoft.com/en-us/windows/win32/api/ws2ipdef/ns-ws2ipdef-sockaddr_in6_lh)
	//
	//
	//  typedef struct _EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP4
	//  {
	//  	0:  uint64_t connectionObj;
	//  	8:  uint32_t localAddrLength;
	//  	12: uint16_t localSinFamily;
	//  	14: uint16_t localPort;
	//  	16: uint32_t localPadding_sin6_flowinfo;
	//  	20: uint16_t localIpAddress[8];
	//      36: uint32_t localPadding_sin6_scope_id;
	//  	40: uint32_t remoteAddrLength;
	//  	44: uint16_t remoteSinFamily;
	//  	46: uint16_t remotePort;
	//  	48: uint32_t remotePadding_sin6_flowinfo;
	//  	52: uint16_t remoteIpAddress[8];
	//      68: uint32_t remotePadding_sin6_scope_id;
	//      72:
	//  } EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP4;

	userData := etw.GoBytes(unsafe.Pointer(eventInfo.Event.UserData), int(eventInfo.Event.UserDataLength))

	// Check for size
	if eventInfo.Event.UserDataLength < 48 {
		log.Errorf("*** Error: User data length for EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn is too small %v\n\n", uintptr(eventInfo.Event.UserDataLength))
		return
	}

	localAddrLength := binary.LittleEndian.Uint32(userData[8:12])
	if localAddrLength != 16 && localAddrLength != 28 {
		log.Errorf("*** Error: ActivityId:%v. HTTPConnectionTraceTaskConnConn invalid local address size %v (only 16 or 28 allowed)\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), localAddrLength)
		return
	}

	var connOpen ConnOpen
	// we're _always_ the server
	if localAddrLength == 16 {
		remoteAddrLength := binary.LittleEndian.Uint32(userData[28:32])
		if remoteAddrLength != 16 {
			log.Errorf("*** Error: ActivityId:%v. HTTPConnectionTraceTaskConnConn invalid remote address size %v (only 16 allowed)\n\n",
				etw.FormatGUID(eventInfo.Event.ActivityId), localAddrLength)
		}

		// Local and remote ipaddress and port
		connOpen.conn.tup.Family = binary.LittleEndian.Uint16(userData[12:14])
		connOpen.conn.tup.LocalPort = binary.BigEndian.Uint16(userData[14:16])
		copy(connOpen.conn.tup.LocalAddr[:], userData[16:20])
		connOpen.conn.tup.RemotePort = binary.BigEndian.Uint16(userData[34:36])
		copy(connOpen.conn.tup.RemoteAddr[:], userData[36:40])
	} else {
		if eventInfo.Event.UserDataLength < 72 {
			log.Errorf("*** Error: User data length for EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn is too small for IP6 %v\n\n", uintptr(eventInfo.Event.UserDataLength))
			return
		}

		remoteAddrLength := binary.LittleEndian.Uint32(userData[40:44])
		if remoteAddrLength != 28 {
			log.Errorf("*** Error: ActivityId:%v. HTTPConnectionTraceTaskConnConn invalid remote address size %v (only 16 allowed)\n\n",
				etw.FormatGUID(eventInfo.Event.ActivityId), localAddrLength)
		}

		//  	20: uint16_t localIpAddress[8];
		//  	46: uint16_t remotePort;
		//  	52: uint16_t remoteIpAddress[8];
		connOpen.conn.tup.Family = binary.LittleEndian.Uint16(userData[12:14])
		connOpen.conn.tup.LocalPort = binary.BigEndian.Uint16(userData[14:16])
		copy(connOpen.conn.tup.LocalAddr[:], userData[20:36])
		connOpen.conn.tup.RemotePort = binary.BigEndian.Uint16(userData[46:48])
		copy(connOpen.conn.tup.RemoteAddr[:], userData[52:68])
	}

	// Time
	connOpen.conn.connected = winutil.FileTimeToUnixNano(uint64(eventInfo.Event.TimeStamp))

	// Http back links (to cleanup on closure)
	connOpen.httpPendingBackLinks = make(map[etw.DDGUID]struct{}, 10)

	// Save to the map
	connOpened[eventInfo.Event.ActivityId] = &connOpen

	// output details
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  Time:           %v\n", etw.FormatUnixTime(connOpen.conn.connected))
		log.Infof("  ActivityId:     %v\n", etw.FormatGUID(eventInfo.Event.ActivityId))
		log.Infof("  Local:          %v\n", etw.IPFormat(connOpen.conn.tup, true))
		log.Infof("  Remote:         %v\n", etw.IPFormat(connOpen.conn.tup, false))
		log.Infof("  Family:         %v\n", connOpen.conn.tup.Family)
		log.Infof("\n")
	}
}

// -------------------------------------------------------------
// HttpService ETW Event #23 (HTTPConnectionTraceTaskConnClose)
func httpCallbackOnHTTPConnectionTraceTaskConnClose(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}

	// output details
	connOpen, found := connOpened[eventInfo.Event.ActivityId]
	if found {
		// ... and clean it up related containers
		delete(http2openConn, eventInfo.Event.ActivityId)

		completedRequestCount++

		// move it to close connection
		connOpen.conn.disconnected = winutil.FileTimeToUnixNano(uint64(eventInfo.Event.TimeStamp))

		// Clean pending http2openConn
		for httpReqRespActivityId := range connOpen.httpPendingBackLinks {
			delete(http2openConn, httpReqRespActivityId)
		}

		// ... and remoe itself from the map
		delete(connOpened, eventInfo.Event.ActivityId)
	}

	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		if found {
			log.Infof("  ActivityId: %v, Local[%v], Remote[%v], Family[%v]\n",
				etw.FormatGUID(eventInfo.Event.ActivityId),
				etw.IPFormat(connOpen.conn.tup, true),
				etw.IPFormat(connOpen.conn.tup, false),
				connOpen.conn.tup.Family)
		} else {
			log.Infof("  ActivityId: %v not found\n", etw.FormatGUID(eventInfo.Event.ActivityId))
		}
		log.Infof("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #1 (HTTPRequestTraceTaskRecvReq)
func httpCallbackOnHTTPRequestTraceTaskRecvReq(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}

	// 	typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4
	// 	{
	// 		0:  uint64_t requestId;
	// 		8:  uint64_t connectionId;
	//      16: uint32_t remoteAddrLength; (or maybe uint16_t, see warning below)
	//      20: uint16_t remoteSinFamily;
	//      22: uint16_t remotePort;
	// 		24: uint32_t remoteIpAddress;
	//      28: uint64_t remoteZeroPad;
	//      36
	// 	} EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4;
	// userData := goBytes(unsafe.Pointer(eventInfo.Event.UserData), C.int(eventInfo.Event.UserDataLength))

	// Check for size
	/*
			 * WARNING
			 *
			 * the format of the UserData structure seemed to magically change for Server 2022
			 * So the expected UserDataLength is 34 (or 44 for ipv6) for 22, and 36/46 for <= 2019
			 *
			 * since we don't use the UserData in this callback, it is safe to skip the previously
			 * implemented length check.
			 *
			 * however, the _warning_ is that if you wish to _use_ the UserData structure, it must
			 * be specially parsed depending on OS version to figure out which byte-packing MS used.
			 *
			 * Specifically, the remoteAddrLength member of the userdata structure went from
			 * 32 bits to 16 bits.  Which is fine, because it's a small number (16 for ipv6).  But
			 * the parsing becomes wonky.
			 *
			 * Suggested check
			 remoteAddrLengthAs32 := binary.LittleEndian.Uint32(userData[16:20])
			 var remoteAddrLengthAs16 uint16
			 parseStart := 20
			 if remoteAddrLengthAs32 > 16 {
				// the remoteAddrLength is packed as a 16 bit int
				remoteAddrLengthAs16 = binary.LittleEndian.Uint16((userData[16:18]))
				parseStart = 18
			 }
		     remoteSinFamily := binary.LittleEndian.Uint16[parseStart:parseStart + 2]
			 remoteSinPort := binary.LittleEndian.Uint16[parseStart + 2:parseStart + 4]

			 * etc....
	*/

	// related activityid
	if eventInfo.RelatedActivityId == nil {
		parsingErrorCount++
		log.Warnf("* Warning!!!: ActivityId:%v. HTTPRequestTraceTaskRecvReq event should have a reference to related activity id\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId))
		return
	}

	connOpen, connFound := getConnOpen(eventInfo.Event.ActivityId)
	if !connFound {
		return
	}

	// Initialize ReqResp and Conn Link
	reqRespAndLink := &HttpConnLink{}
	reqRespAndLink.connActivityId = eventInfo.Event.ActivityId
	reqRespAndLink.http.Txn.Tup = connOpen.conn.tup
	reqRespAndLink.http.Txn.RequestStarted = winutil.FileTimeToUnixNano(uint64(eventInfo.Event.TimeStamp))

	// Save Req/Resp Conn Link and back reference to it
	http2openConn[*eventInfo.RelatedActivityId] = reqRespAndLink
	var dummy struct{}
	connOpen.httpPendingBackLinks[*eventInfo.RelatedActivityId] = dummy

	// output details
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  Time:           %v\n", etw.FormatUnixTime(reqRespAndLink.http.Txn.RequestStarted))
		log.Infof("  ActivityId:     %v\n", etw.FormatGUID(eventInfo.Event.ActivityId))
		log.Infof("  RelActivityId:  %v\n", etw.FormatGUID(*eventInfo.RelatedActivityId))
		if connFound {
			log.Infof("  Local:          %v\n", etw.IPFormat(connOpen.conn.tup, true))
			log.Infof("  Remote:         %v\n", etw.IPFormat(connOpen.conn.tup, false))
			log.Infof("  Family:         %v\n", connOpen.conn.tup.Family)
		}
		log.Infof("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #2 (HTTPRequestTraceTaskParse)
func httpCallbackOnHTTPRequestTraceTaskParse(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}

	// typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskParse
	// {
	// 	    0:  uint64_t requestObj;
	// 	    8:  uint32_t httpVerb;
	// 	    12: unint8_t url;           // Unicode wide char zero terminating string
	// } EVENT_PARAM_HttpService_HTTPRequestTraceTaskParse;
	userData := etw.GoBytes(unsafe.Pointer(eventInfo.Event.UserData), int(eventInfo.Event.UserDataLength))

	// Check for size
	if eventInfo.Event.UserDataLength < 14 {
		parsingErrorCount++
		log.Errorf("*** Error: ActivityId:%v. User data length for HTTPRequestTraceTaskParse is too small %v\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), uintptr(eventInfo.Event.UserDataLength))
		return
	}

	// Get req/resp conn link
	httpConnLink, found := getHttpConnLink(eventInfo.Event.ActivityId)
	if !found {
		return
	}

	// Verb (in future we can cast number to)
	httpConnLink.http.Txn.RequestMethod = etw.VerbToMethod(binary.LittleEndian.Uint32(userData[8:12]))

	// Parse Url
	urlOffset := 12
	uri, _, urlFound, urlTermZeroIdx := etw.ParseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		log.Errorf("*** Error: ActivityId:%v. HTTPRequestTraceTaskParse could not find terminating zero for Url. termZeroIdx=%v\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), urlTermZeroIdx)

		// If problem stop tracking and cleanup
		cleanupActivityIdViaConnActivityId(httpConnLink.connActivityId, eventInfo.Event.ActivityId)
		return
	}

	// <<<MORE ETW HttpService DETAILS>>>
	// httpConnLink.http.Url = uri
	httpConnLink.url = uri

	// Parse url (manual persing may be will be bit faster, we need like find 3 "/")
	urlParsed, err := url.Parse(uri)
	if err == nil {
		if len(urlParsed.Path) == 0 {
			urlParsed.Path = "/"
		}
		// httpConnLink.http.RequestFragment[0] = 32 is done to simulate
		//   func getPath(reqFragment, buffer []byte) []byte
		// which expects something like "GET /foo?var=bar HTTP/1.1"
		// in future it probably should be optimize because we have already
		// whole thing
		httpConnLink.http.RequestFragment = make([]byte, maxRequestFragmentBytes)
		httpConnLink.http.Txn.MaxRequestFragment = uint16(maxRequestFragmentBytes)
		httpConnLink.http.RequestFragment[0] = 32 // this is a leading space.

		// copy rest of arguments
		copy(httpConnLink.http.RequestFragment[1:], urlParsed.Path)

	}

	// output details
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  ActivityId:     %v\n", etw.FormatGUID(eventInfo.Event.ActivityId))
		log.Infof("  Url:            %v\n", httpConnLink.url)
		log.Infof("  Method:         %v\n", etw.HTTPMethodToStr(httpConnLink.http.Txn.RequestMethod))
		log.Infof("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #3 (HTTPRequestTraceTaskDeliver)
func httpCallbackOnHTTPRequestTraceTaskDeliver(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
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
	userData := etw.GoBytes(unsafe.Pointer(eventInfo.Event.UserData), int(eventInfo.Event.UserDataLength))

	// Check for size
	if eventInfo.Event.UserDataLength < 24 {
		parsingErrorCount++
		log.Errorf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskDeliver is too small %v\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), uintptr(eventInfo.Event.UserDataLength))
		return
	}

	// Get req/resp conn link
	httpConnLink, found := getHttpConnLink(eventInfo.Event.ActivityId)
	if !found {
		return
	}

	// Extra output
	connOpen, connFound := getConnOpen(httpConnLink.connActivityId)
	if !connFound {
		// If no connection found then stop tracking
		delete(http2openConn, eventInfo.Event.ActivityId)
		return
	}

	// Parse RequestQueueName
	appPoolOffset := 20
	appPool, urlOffset, appPoolFound, appPoolTermZeroIdx := etw.ParseUnicodeString(userData, appPoolOffset)
	if !appPoolFound {
		parsingErrorCount++
		log.Errorf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), etw.FormatGUID(httpConnLink.connActivityId), appPoolTermZeroIdx)

		// If problem stop tracking this
		delete(http2openConn, eventInfo.Event.ActivityId)
		return
	}

	httpConnLink.http.AppPool = appPool
	httpConnLink.http.SiteID = binary.LittleEndian.Uint32(userData[16:24])
	httpConnLink.http.SiteName = iisConfig.GetSiteNameFromID(httpConnLink.http.SiteID)
	// Parse url
	if urlOffset > len(userData) {
		parsingErrorCount++

		log.Errorf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskDeliver could not find beginning of Url\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), etw.FormatGUID(httpConnLink.connActivityId))

		// If problem stop tracking this
		delete(http2openConn, eventInfo.Event.ActivityId)
		return
	}

	// output details
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  ConnActivityId: %v\n", etw.FormatGUID(httpConnLink.connActivityId))
		log.Infof("  ActivityId:     %v\n", etw.FormatGUID(eventInfo.Event.ActivityId))
		log.Infof("  AppPool:        %v\n", httpConnLink.http.AppPool)
		log.Infof("  Url:            %v\n", httpConnLink.url)
		if connFound {
			log.Infof("  Local:          %v\n", etw.IPFormat(connOpen.conn.tup, true))
			log.Infof("  Remote:         %v\n", etw.IPFormat(connOpen.conn.tup, false))
			log.Infof("  Family:         %v\n", connOpen.conn.tup.Family)
		}
		log.Infof("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #4, #8 (HTTPRequestTraceTaskFastResp, HTTPRequestTraceTaskRecvResp)
func httpCallbackOnHTTPRequestTraceTaskRecvResp(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
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

	userData := etw.GoBytes(unsafe.Pointer(eventInfo.Event.UserData), int(eventInfo.Event.UserDataLength))

	// Check for size
	if eventInfo.Event.UserDataLength < 24 {
		parsingErrorCount++
		log.Errorf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskXxxResp is too small %v\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), uintptr(eventInfo.Event.UserDataLength))
		return
	}

	// Get req/resp conn link
	httpConnLink, found := getHttpConnLink(eventInfo.Event.ActivityId)
	if !found {
		return
	}
	httpConnLink.http.Txn.ResponseStatusCode = binary.LittleEndian.Uint16(userData[16:18])

	// <<<MORE ETW HttpService DETAILS>>>
	// verbOffset := 18
	// headerSizeOffset, verbFound, verbTermZeroIdx := skipAsciiString(userData, verbOffset)
	//if !verbFound {
	//	parsingErrorCount++
	//	log.Errorf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskXxxResp could not find terminating zero for Verb. termZeroIdx=%v\n\n",
	//		formatGuid(eventInfo.Event.ActivityId), etw.FormatGuid(httpConnLink.connActivityId), verbTermZeroIdx)
	//	return
	//}

	// <<<MORE ETW HttpService DETAILS>>>
	// // Parse headerLength (space for 32bit number)
	// if (headerSizeOffset + 4) > len(userData) {
	// 	log.Errorf("*** Error: ActivityId:%v. Connection ActivityId:%v. HTTPRequestTraceTaskXxxResp Not enough space for HeaderLength. userDataSize=%v, parsedDataSize=%v\n\n",
	//  	formatGuid(eventInfo.Event.ActivityId), etw.FormatGuid(httpConnLink.connActivityId), len(userData), (headerSizeOffset + 4))
	//	return
	//}

	// <<<MORE ETW HttpService DETAILS>>>
	// httpConnLink.http.HeaderLength = binary.LittleEndian.Uint32(userData[headerSizeOffset:])
}

// -----------------------------------------------------------
// HttpService ETW Event #16-17 (HTTPRequestTraceTaskSrvdFrmCache, HTTPRequestTraceTaskCachedNotModified)
func httpCallbackOnHTTPRequestTraceTaskSrvdFrmCache(eventInfo *etw.DDEtwEventInfo) {

	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}

	// typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache
	// {
	// 	   0:  uint64_t requestObj;
	// 	   8:  uint32_t SiteId;
	// 	   12: uint32_t bytesSent;
	// } EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache;

	// userData := goBytes(unsafe.Pointer(eventInfo.Event.UserData), C.int(eventInfo.Event.UserDataLength))

	// Check for size
	if eventInfo.Event.UserDataLength < 12 {
		parsingErrorCount++
		log.Errorf("*** Error: ActivityId:%v. User data length for EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache is too small %v\n\n",
			etw.FormatGUID(eventInfo.Event.ActivityId), uintptr(eventInfo.Event.UserDataLength))
		return
	}

	// Get req/resp conn link
	httpConnLink, found := getHttpConnLink(eventInfo.Event.ActivityId)
	if !found {
		return
	}

	// Get from HTTP.sys cache (httpCache)
	cacheEntry, found := httpCache[httpConnLink.url]
	if !found {
		log.Warnf("* Warning!!!: HTTPRequestTraceTaskSrvdFrmCache failed to find HTTP cache entry by url %v\n\n", httpConnLink.url)

		// If problem stop tracking and cleanup
		cleanupActivityIdViaConnActivityId(httpConnLink.connActivityId, eventInfo.Event.ActivityId)
		return
	}

	// Log the findings
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  Cache entry for %v is found\n", httpConnLink.url)
		if cacheEntry.cache.reqRespBound {
			log.Infof("  Completing reqResp tracking\n")
		} else {
			log.Infof("  Updating cache entry via current http request\n")
		}
		log.Infof("\n")
	}

	if cacheEntry.cache.reqRespBound {
		// Get from cache and complete reqResp tracking
		httpConnLink.http = cacheEntry.http
		httpConnLink.http.AppPool = cacheEntry.http.AppPool
		httpConnLink.http.Txn.ResponseStatusCode = cacheEntry.http.Txn.ResponseStatusCode

		// <<<MORE ETW HttpService DETAILS>>>
		httpConnLink.http.SiteID = cacheEntry.http.SiteID
		httpConnLink.http.SiteName = iisConfig.GetSiteNameFromID(cacheEntry.http.SiteID)

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
func httpCallbackOnHTTPCacheTraceTaskAddedCacheEntry(eventInfo *etw.DDEtwEventInfo) {

	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
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

	userData := etw.GoBytes(unsafe.Pointer(eventInfo.Event.UserData), int(eventInfo.Event.UserDataLength))

	cacheEntry := &HttpCache{}

	// Parse Url
	urlOffset := 0
	url, statusCodeOffset, urlFound, urlTermZeroIdx := etw.ParseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		log.Errorf("*** Error: HTTPCacheTraceTaskAddedCacheEntry could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n", urlTermZeroIdx)
		return
	}

	// Status code
	cacheEntry.cache.statusCode = binary.LittleEndian.Uint16(userData[statusCodeOffset : statusCodeOffset+2])

	// <<<MORE ETW HttpService DETAILS>>>
	// // Parse Verb
	// verbOffset := statusCodeOffset + 2
	// verb, headerSizeOffset, verbFound, verbTermZeroIdx := parseAsciiString(userData, verbOffset)
	// if !verbFound {
	//	parsingErrorCount++
	//	log.Errorf("*** Error: HTTPCacheTraceTaskAddedCacheEntry could not find terminating zero for Verb. termZeroIdx=%v\n\n", verbTermZeroIdx)
	//	return
	//}
	//cacheEntry.cache.verb = verb

	// <<<MORE ETW HttpService DETAILS>>>
	//	// Parse headerLength (space for 32bit number)
	// if (headerSizeOffset + 4) > len(userData) {
	// 	log.Errorf("*** Error: HTTPCacheTraceTaskAddedCacheEntry Not enough space for HeaderLength. userDataSize=%v, parsedDataSize=%v\n\n",
	// 		len(userData), (headerSizeOffset + 4))
	// 	return
	// }
	// cacheEntry.cache.headerLength = binary.LittleEndian.Uint32(userData[headerSizeOffset:])

	// <<<MORE ETW HttpService DETAILS>>>
	// // Parse contentLength (space for 32bit number)
	// contentLengthOffset := headerSizeOffset + 4
	// if (contentLengthOffset + 4) > len(userData) {
	// 	log.Errorf("*** Error: HTTPCacheTraceTaskAddedCacheEntry Not enough space for contentLengthOffset. userDataSize=%v, parsedDataSize=%v\n\n",
	// 		len(userData), (contentLengthOffset + 4))
	// 	return
	// }
	// cacheEntry.cache.contentLength = binary.LittleEndian.Uint32(userData[contentLengthOffset:])

	cacheEntry.cache.reqRespBound = false

	// Save it to sysCache
	httpCache[url] = cacheEntry

	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		log.Infof("  Url:            %v\n", url)
		log.Infof("  StatusCode:     %v\n", cacheEntry.cache.statusCode)
		// <<<MORE ETW HttpService DETAILS>>>
		// log.Infof("  Verb:           %v\n", cacheEntry.cache.verb)
		// log.Infof("  HeaderLength:   %v\n", cacheEntry.cache.headerLength)
		// log.Infof("  ContentLength:  %v\n", cacheEntry.cache.contentLength)
		log.Infof("\n")
	}
}

// -----------------------------------------------------------
// HttpService ETW Event #26 (HTTPCacheTraceTaskFlushedCache)
func httpCallbackOnHTTPCacheTraceTaskFlushedCache(eventInfo *etw.DDEtwEventInfo) {

	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
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

	userData := etw.GoBytes(unsafe.Pointer(eventInfo.Event.UserData), int(eventInfo.Event.UserDataLength))

	// Parse Url
	urlOffset := 0
	url, _, urlFound, urlTermZeroIdx := etw.ParseUnicodeString(userData, urlOffset)
	if !urlFound {
		parsingErrorCount++
		log.Errorf("*** Error: HTTPCacheTraceTaskFlushedCache could not find terminating zero for RequestQueueName. termZeroIdx=%v\n\n", urlTermZeroIdx)
		return
	}

	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		cacheEntry, found := httpCache[url]
		if !found {
			missedCacheCount++
			log.Warnf("* Warning!!!: HTTPCacheTraceTaskFlushedCache failed to find cached url %v\n\n", url)
			return
		}

		log.Infof("  Url:            %v\n", url)
		log.Infof("  StatusCode:     %v\n", cacheEntry.cache.statusCode)
		// <<<MORE ETW HttpService DETAILS>>>
		// log.Infof("  Verb:           %v\n", cacheEntry.cache.verb)
		// log.Infof("  HeaderLength:   %v\n", cacheEntry.cache.headerLength)
		// log.Infof("  ContentLength:  %v\n", cacheEntry.cache.contentLength)

		if cacheEntry.cache.reqRespBound {
			// <<<MORE ETW HttpService DETAILS>>>
			// log.Infof("  SiteID:         %v\n", cacheEntry.http.SiteID)

			log.Infof("  AppPool:        %v\n", cacheEntry.http.AppPool)
		}

		log.Infof("\n")
	}

	// Delete it from sysCache
	delete(httpCache, url)
}

// -----------------------------------------------------------
// HttpService ETW Event #10-14 (HTTPRequestTraceTaskXXXSendXXX)
func httpCallbackOnHTTPRequestTraceTaskSend(eventInfo *etw.DDEtwEventInfo) {

	// We probably should use this event as a last event for a particular activity and use
	// it to better measure duration is http procesing
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}

	// Get req/resp conn link
	httpConnLink, found := getHttpConnLink(eventInfo.Event.ActivityId)
	if !found {
		return
	}

	completeReqRespTracking(eventInfo, httpConnLink)
}

// -----------------------------------------------------------
// HttpService ETW Event #34 (EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent)
func httpCallbackOnHttpSslConnEvent(eventInfo *etw.DDEtwEventInfo) {
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, true)
	}
	/*
			typedef struct _EVENT_PARAM_HttpService_HTTPTraceTaskConnCleanup {
		    	0: uint64_t connectionObj;
			} EVENT_PARAM_HttpService_HTTPTraceTaskConnCleanup;
	*/
	if !captureHTTPS {

		if HttpServiceLogVerbosity != HttpServiceLogVeryVerbose {
			// Drop it immediately ...
			delete(connOpened, eventInfo.Event.ActivityId)
		} else {
			// ... unless  if we want to track to the very end
			connOpen, found := connOpened[eventInfo.Event.ActivityId]
			if found {
				connOpen.ssl = true
			}

		}
	}
}
func reportHttpCallbackEvents(eventInfo *etw.DDEtwEventInfo, willBeProcessed bool) {
	var processingStatus string
	if willBeProcessed {
		processingStatus = "processing"
	} else {
		processingStatus = "skipped"
	}

	log.Infof("Http-service (%v) Id:%v/%v, level:%v, opcode:%v, task:%v, keyword:0x%x, seq:%v\n",
		processingStatus, eventInfo.Event.Id, etw.FormatHTTPServiceEventID(uint16(eventInfo.Event.Id)),
		eventInfo.Event.Level, eventInfo.Event.Opcode, eventInfo.Event.Task, eventInfo.Event.Keyword,
		eventCount)
}

func httpCallbackOnHttpServiceNonProcessedEvents(eventInfo *etw.DDEtwEventInfo) {
	notHandledEventsCount++
	if HttpServiceLogVerbosity == HttpServiceLogVeryVerbose {
		reportHttpCallbackEvents(eventInfo, false)
		log.Infof("\n)")
	}
}

func etwHttpServiceSummary() {
	lastSummaryTime = time.Now()
	summaryCount++

	log.Debugf("=====================\n")
	log.Debugf("  SUMMARY #%v\n", summaryCount)
	log.Debugf("=====================\n")
	log.Debugf("  Pid:                      %v\n", os.Getpid())
	log.Debugf("  Conn map:                 %v\n", len(connOpened))
	log.Debugf("  Req/Resp map:             %v\n", len(http2openConn))
	log.Debugf("  Cache map:                %v\n", len(httpCache))
	log.Debugf("  All Events(not handled):  %v(%v)\n", etw.FormatUInt(eventCount), etw.FormatUInt(notHandledEventsCount))
	log.Debugf("  Requests(cached):         %v(%v)\n", etw.FormatUInt(completedRequestCount), etw.FormatUInt(servedFromCache))
	log.Debugf("  Missed Conn:              %v\n", etw.FormatUInt(missedConnectionCount))
	log.Debugf("  Parsing Error:            %v\n", etw.FormatUInt(parsingErrorCount))
	log.Debugf("  ETW Bytes Total(Payload): %v(%v)\n", etw.BytesFormat(transferedETWBytesTotal), etw.BytesFormat(transferedETWBytesPayload))
	log.Debugf("  Dropped Tx (Limit):       %v(%v)\n", completedHttpTxDropped, completedHttpTxMaxCount)

	/*
		* dbtodo
		*
		* gopsutil on Windows causes bad things (WMI).  Decide if we need this info
		* and if so get another way.
		if curProc, err := process.NewProcess(int32(os.Getpid())); err == nil {
			if cpu, err2 := curProc.CPUPercent(); err2 == nil {
				log.Infof("  CPU:                      %.2f%%\n", cpu)
			}

			if memInfo, err2 := curProc.MemoryInfo(); err2 == nil {
				log.Infof("  VMS(RSS):                 %v(%v)\n", bytesFormat(memInfo.VMS), bytesFormat(memInfo.RSS))
			}
		}

		fmt.Print("\n")
	*/
}

func (hei *EtwInterface) OnEvent(eventInfo *etw.DDEtwEventInfo) {

	// Total number of bytes transferred to kernel from HTTP.sys driver. 0x68 is ETW header size
	transferedETWBytesTotal += (uint64(eventInfo.Event.UserDataLength) + 0x68)
	transferedETWBytesPayload += uint64(eventInfo.Event.UserDataLength)

	eventCount++

	switch eventInfo.Event.Id {
	// #21
	case etw.EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn:
		httpCallbackOnHTTPConnectionTraceTaskConnConn(eventInfo)

	// #23
	case etw.EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose:
		httpCallbackOnHTTPConnectionTraceTaskConnClose(eventInfo)

	// #1
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq:
		httpCallbackOnHTTPRequestTraceTaskRecvReq(eventInfo)

	// #2
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskParse:
		httpCallbackOnHTTPRequestTraceTaskParse(eventInfo)

	// #3
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver:
		httpCallbackOnHTTPRequestTraceTaskDeliver(eventInfo)

	// #4, #8
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp:
		fallthrough
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp:
		httpCallbackOnHTTPRequestTraceTaskRecvResp(eventInfo)

	// #16, #17
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache:
		fallthrough
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified:
		httpCallbackOnHTTPRequestTraceTaskSrvdFrmCache(eventInfo)

	// #25
	case etw.EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry:
		httpCallbackOnHTTPCacheTraceTaskAddedCacheEntry(eventInfo)

	// #27
	case etw.EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache:
		httpCallbackOnHTTPCacheTraceTaskFlushedCache(eventInfo)

	// #34
	case etw.EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent:
		httpCallbackOnHttpSslConnEvent(eventInfo)

	// #10-14
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete:
		fallthrough
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend:
		fallthrough
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend:
		fallthrough
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend:
		fallthrough
	case etw.EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError:
		httpCallbackOnHTTPRequestTraceTaskSend(eventInfo)

	default:
		httpCallbackOnHttpServiceNonProcessedEvents(eventInfo)
	}

	// output summary every 40 seconds
	if HttpServiceLogVerbosity != HttpServiceLogNone {
		if time.Since(lastSummaryTime).Seconds() >= 30 {
			etwHttpServiceSummary()
		}
	}
}

// can be called multiple times
func initializeEtwHttpServiceSubscription() {
	connOpened = make(map[etw.DDGUID]*ConnOpen)
	http2openConn = make(map[etw.DDGUID]*HttpConnLink)
	httpCache = make(map[string]*HttpCache, 100)

	summaryCount = 0
	eventCount = 0
	servedFromCache = 0
	completedRequestCount = 0
	missedConnectionCount = 0
	missedCacheCount = 0
	parsingErrorCount = 0
	notHandledEventsCount = 0
	transferedETWBytesTotal = 0
	transferedETWBytesPayload = 0

	lastSummaryTime = time.Now()

	completedHttpTxMux.Lock()
	defer completedHttpTxMux.Unlock()
	completedHttpTx = make([]WinHttpTransaction, 0, 100)
}

func (h *Http) String() string {
	var output strings.Builder
	output.WriteString("httpTX{")
	output.WriteString("Method: '" + strconv.Itoa(int(h.Txn.RequestMethod)) + "', ")
	//output.WriteString("Fragment: '" + hex.EncodeToString(tx.RequestFragment[:]) + "', ")
	output.WriteString("\n  Fragment: '" + string(h.RequestFragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}
func ReadHttpTx() (httpTxs []WinHttpTransaction, err error) {
	if !httpServiceSubscribed {
		return nil, errors.New("ETW HttpService is not currently subscribed")
	}

	completedHttpTxMux.Lock()
	defer completedHttpTxMux.Unlock()

	// Return accumulated httpTx and reset array
	readHttpTx := completedHttpTx

	completedHttpTx = make([]WinHttpTransaction, 0, 100)

	return readHttpTx, nil
}

func SetMaxFlows(maxFlows uint64) {
	completedHttpTxMaxCount = maxFlows
}

func SetMaxRequestBytes(maxRequestBytes uint64) {
	maxRequestFragmentBytes = maxRequestBytes
}

func SetEnabledProtocols(http, https bool) {
	captureHTTP = http
	captureHTTPS = https
}
func (hei *EtwInterface) OnStart() {
	initializeEtwHttpServiceSubscription()
	httpServiceSubscribed = true
	var err error
	iisConfig, err = winutil.NewDynamicIISConfig()
	if err != nil {
		log.Warnf("Failed to create iis config %v", err)
		iisConfig = nil
	} else {
		err = iisConfig.Start()
		if err != nil {
			log.Warnf("Failed to start iis config %v", err)
			iisConfig = nil
		}
	}
}

func (hei *EtwInterface) OnStop() {
	httpServiceSubscribed = false
	initializeEtwHttpServiceSubscription()
	if iisConfig != nil {
		iisConfig.Stop()
		iisConfig = nil
	}
}
