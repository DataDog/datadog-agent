// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//gobuild ignore
//go:build ignore

package etw

/*
#include "etw.h"
#include "etw-provider.h"
*/
import "C"

const (
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn       = C.EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn
	EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose      = C.EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose
	EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq           = C.EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq
	EVENT_ID_HttpService_HTTPRequestTraceTaskParse             = C.EVENT_ID_HttpService_HTTPRequestTraceTaskParse
	EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver           = C.EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver
	EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp          = C.EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp
	EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp          = C.EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp
	EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache      = C.EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache
	EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified = C.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified
	EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry     = C.EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry
	EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache        = C.EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache
	EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent          = C.EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent
	EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete      = C.EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete
	EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend     = C.EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend
	EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend          = C.EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend
	EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend          = C.EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend
	EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError      = C.EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError
)

const (
	// these defines are bitmasks
	DD_ETW_TRACE_PROVIDER_HttpService int64 = C.DD_ETW_TRACE_PROVIDER_HttpService
	DD_ETW_TRACE_PROVIDER_TCPIP             = C.DD_ETW_TRACE_PROVIDER_TCPIP
	DD_ETW_TRACE_PROVIDER_UDP               = C.DD_ETW_TRACE_PROVIDER_UDP
	DD_ETW_TRACE_PROVIDER_DNS               = C.DD_ETW_TRACE_PROVIDER_DNS
)

type DDGUID C.DDGUID
type DDEtwEvent C.DD_ETW_EVENT
type DDEtwEventInfo C.DD_ETW_EVENT_INFO
