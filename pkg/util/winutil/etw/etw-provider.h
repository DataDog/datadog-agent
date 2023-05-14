#ifndef __ETW_PROVIDER_H
#define __ETW_PROVIDER_H

// Provided specific commands and structure

#pragma pack(push)
#pragma pack(1)

// ==============================================================================
//
// Microsoft-Windows-HttpService  {dd5ef90a-6398-47a4-ad34-4dcecdef795f}
//
//     https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-18990/Microsoft-Windows-HttpService.xml


// ------------------------------------------------------------------------------
// <event value="1" symbol="HTTPRequestTraceTaskRecvReq" version="0" task="HTTPRequestTraceTask" opcode="RecvReq" level="win:Informational" keywords="HTTP_KEYWORD_REQUEST HTTP_KEYWORD_REQUEST_QUEUE" template="HTTPRequestTraceTaskRecvReqArgs"/>
//
//     <template tid="HTTPRequestTraceTaskRecvReqArgs">
//      <data name="RequestId" inType="win:UInt64"/>
//      <data name="ConnectionId" inType="win:UInt64"/>
//      <data name="RemoteAddrLength" inType="win:UInt32"/>
//      <data name="RemoteAddr" inType="win:Binary" length="RemoteAddrLength"/>
//     </template>
//
#define EVENT_ID_HttpService_HTTPRequestTraceTaskRecvReq  1
typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4
{
    uint64_t requestId;
    uint64_t connectionId;
    uint32_t remoteAddrLength;
    uint16_t remoteSinFamily;
    uint16_t remotePort;
    uint32_t remoteIpAddress;
    uint64_t remoteZeroPad;
} EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvReq_IP4;


// ------------------------------------------------------------------------------
// <event value="2" symbol="HTTPRequestTraceTaskParse" version="0" task="HTTPRequestTraceTask" opcode="Parse" level="win:Informational" keywords="HTTP_KEYWORD_REQUEST" template="HTTPRequestTraceTaskParseArgs"/>
//
//     <template tid="HTTPRequestTraceTaskParseArgs">
//      <data name="RequestObj" inType="win:Pointer"/>
//      <data name="HttpVerb" inType="win:UInt32"/>
//      <data name="Url" inType="win:UnicodeString"/>
//     </template>

#define EVENT_ID_HttpService_HTTPRequestTraceTaskParse  2
typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskParse
{
    uint64_t requestObj;
    uint32_t httpVerb;
    uint8_t  url;           // Unicode wide char zero terminating string
} EVENT_PARAM_HttpService_HTTPRequestTraceTaskParse;

// ------------------------------------------------------------------------------
// <event value="3" symbol="HTTPRequestTraceTaskDeliver" version="0" task="HTTPRequestTraceTask" opcode="Deliver" level="win:Informational" keywords="HTTP_KEYWORD_REQUEST HTTP_KEYWORD_REQUEST_QUEUE" template="HTTPRequestTraceTaskDeliverArgs"/>
//
//     <template tid="HTTPRequestTraceTaskDeliverArgs">
//      <data name="RequestObj" inType="win:Pointer"/>
//      <data name="RequestId" inType="win:UInt64"/>
//      <data name="SiteId" inType="win:UInt32"/>
//      <data name="RequestQueueName" inType="win:UnicodeString"/>
//      <data name="Url" inType="win:UnicodeString"/>
//      <data name="Status" inType="win:UInt32"/>
//     </template>
//
#define EVENT_ID_HttpService_HTTPRequestTraceTaskDeliver  3
typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskDeliver
{
    uint64_t requestObj;
    uint64_t requestId;
    uint32_t siteId;
    uint8_t  requestQueueName[1]; // Unicode wide char zero terminating string
//  uint8_t  url[1];              // Unicode wide char zero terminating string
//  uint32_t status;
} EVENT_PARAM_HttpService_HTTPRequestTraceTaskDeliver;

// ------------------------------------------------------------------------------
// <event value="4" symbol="HTTPRequestTraceTaskRecvResp" version="0" task="HTTPRequestTraceTask" opcode="RecvResp" level="win:Informational" keywords="HTTP_KEYWORD_REQUEST HTTP_KEYWORD_RESPONSE" template="HTTPRequestTraceTaskRecvRespArgs"/>
// <event value="8" symbol="HTTPRequestTraceTaskFastResp" version="0" task="HTTPRequestTraceTask" opcode="FastResp" level="win:Informational" keywords="HTTP_KEYWORD_REQUEST HTTP_KEYWORD_RESPONSE" template="HTTPRequestTraceTaskRecvRespArgs"/>
//
//     <template tid="HTTPRequestTraceTaskRecvRespArgs">
//      <data name="RequestId" inType="win:UInt64"/>
//      <data name="ConnectionId" inType="win:UInt64"/>
//      <data name="StatusCode" inType="win:UInt16"/>
//      <data name="Verb" inType="win:AnsiString"/>
//      <data name="HeaderLength" inType="win:UInt32"/>
//      <data name="EntityChunkCount" inType="win:UInt16"/>
//      <data name="CachePolicy" inType="win:UInt32"/>
//     </template>
//

#define EVENT_ID_HttpService_HTTPRequestTraceTaskRecvResp  4
#define EVENT_ID_HttpService_HTTPRequestTraceTaskFastResp  8
typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvResp
{
    uint64_t requestId;
    uint64_t connectionId;
    uint16_t statusCode;
    uint8_t  verb[1];      // ASCII zero terminating string string
//  uint32   headerLength
//  uint16_t entityChunkCount;
//  uint32_t cachePolicy
} EVENT_PARAM_HttpService_HTTPRequestTraceTaskRecvResp;

#define EVENT_ID_HttpService_HTTPRequestTraceTaskSendComplete   10
#define EVENT_ID_HttpService_HTTPRequestTraceTaskCachedAndSend  11
#define EVENT_ID_HttpService_HTTPRequestTraceTaskFastSend       12
#define EVENT_ID_HttpService_HTTPRequestTraceTaskZeroSend       13
#define EVENT_ID_HttpService_HTTPRequestTraceTaskLastSndError   14
typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskSend
{
    uint64_t requestId;
    uint16_t httpCode;
} EVENT_PARAM_HttpService_HTTPRequestTraceTaskSend;

// ------------------------------------------------------------------------------
// <event value="16" symbol="HTTPRequestTraceTaskSrvdFrmCache" version="0" task="HTTPRequestTraceTask" opcode="SrvdFrmCache" level="win:Informational" keywords="HTTP_KEYWORD_RESPONSE HTTP_KEYWORD_CACHE" template="HTTPRequestTraceTaskSrvdFrmCacheArgs"/>
// <event value="16" symbol="HTTPRequestTraceTaskSrvdFrmCache_V1" version="1" task="HTTPRequestTraceTask" opcode="SrvdFrmCache" level="win:Informational" keywords="HTTP_KEYWORD_RESPONSE HTTP_KEYWORD_CACHE" template="HTTPRequestTraceTaskSrvdFrmCacheArgs_V1"/>
// <event value="17" symbol="HTTPRequestTraceTaskCachedNotModified" version="0" task="HTTPRequestTraceTask" opcode="CachedNotModified" level="win:Informational" keywords="HTTP_KEYWORD_RESPONSE HTTP_KEYWORD_CACHE" template="HTTPRequestTraceTaskSrvdFrmCacheArgs"/>
// <event value="17" symbol="HTTPRequestTraceTaskCachedNotModified_V1" version="1" task="HTTPRequestTraceTask" opcode="CachedNotModified" level="win:Informational" keywords="HTTP_KEYWORD_RESPONSE HTTP_KEYWORD_CACHE" template="HTTPRequestTraceTaskSrvdFrmCacheArgs_V1"/>
//
//    <template tid="HTTPRequestTraceTaskSrvdFrmCacheArgs_V1">
//      <data name="RequestObj" inType="win:Pointer"/>   v0
//      <data name="SiteId" inType="win:UInt32"/>        v0 
//      <data name="BytesSent" inType="win:UInt32"/>     v0
//      <data name="RequestId" inType="win:UInt64"/>     v1
//      <data name="Encoding" inType="win:AnsiString"/>  v1
//     </template>
#define EVENT_ID_HttpService_HTTPRequestTraceTaskSrvdFrmCache       16
#define EVENT_ID_HttpService_HTTPRequestTraceTaskCachedNotModified  17
typedef struct _EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache
{
    uint64_t requestObj;
    uint32_t SiteId;
    uint32_t bytesSent;
} EVENT_PARAM_HttpService_HTTPRequestTraceTaskSrvdFrmCache;



// ------------------------------------------------------------------------------
// <event value="21" symbol="HTTPConnectionTraceTaskConnConnect" version="0" task="HTTPConnectionTraceTask" opcode="ConnConnect" level="win:Informational" keywords="HTTP_KEYWORD_CONNECTION" template="HTTPConnectionTraceTaskConnConnectArgs"/>
//
//     <template tid="HTTPConnectionTraceTaskConnConnectArgs">
//      <data name="ConnectionObj" inType="win:Pointer"/>
//      <data name="LocalAddrLength" inType="win:UInt32"/>
//      <data name="LocalAddr" inType="win:Binary" length="LocalAddrLength"/>
//      <data name="RemoteAddrLength" inType="win:UInt32"/>
//      <data name="RemoteAddr" inType="win:Binary" length="RemoteAddrLength"/>
//     </template>
//
#define EVENT_ID_HttpService_HTTPConnectionTraceTaskConnConn  21
typedef struct _EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP4
{
    uint64_t connectionObj;
    uint32_t localAddrLength;
    uint16_t localSinFamily;
    uint16_t localPort;          // hton
    uint32_t localIpAddress;
    uint64_t localZeroPad;
    uint32_t remoteAddrLength;
    uint16_t remoteSinFamily;
    uint16_t remotePort;         // hton
    uint32_t remoteIpAddress;
    uint64_t remoteZeroPad;
} EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP4;

typedef struct _EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP6
{
    uint64_t connectionObj;
    uint32_t localAddrLength;
    // TBD
}EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnConnect_IP6;

// ------------------------------------------------------------------------------
// <event value="23" symbol="HTTPConnectionTraceTaskConnClose" version="0" task="HTTPConnectionTraceTask" opcode="ConnClose" level="win:Informational" keywords="HTTP_KEYWORD_CONNECTION" template="HTTPConnectionTraceTaskConnCloseArgs"/>
//
//     <template tid="HTTPConnectionTraceTaskConnCloseArgs">
//      <data name="ConnectionObj" inType="win:Pointer"/>
//      <data name="Abortive" inType="win:UInt32"/>
//     </template>
//
#define EVENT_ID_HttpService_HTTPConnectionTraceTaskConnClose  23
typedef struct _EVENT_PARAM_HTTPConnectionTraceTaskConnClose
{
    uint64_t connectionObj;
    uint32_t abortive;
} EVENT_PARAM_HttpService_HTTPConnectionTraceTaskConnClose;

// ------------------------------------------------------------------------------
// <event value="25" symbol="HTTPCacheTraceTaskAddedCacheEntry" version="0" task="HTTPCacheTraceTask" opcode="AddedCacheEntry" level="win:Informational" keywords="HTTP_KEYWORD_CACHE" template="HTTPCacheTraceTaskAddedCacheEntryArgs"/>
// <event value="25" symbol="HTTPCacheTraceTaskAddedCacheEntry_V1" version="1" task="HTTPCacheTraceTask" opcode="AddedCacheEntry" level="win:Informational" keywords="HTTP_KEYWORD_CACHE" template="HTTPCacheTraceTaskAddedCacheEntryArgs_V1"/>
// <event value="27" symbol="HTTPCacheTraceTaskFlushedCache" version="0" task="HTTPCacheTraceTask" opcode="FlushedCache" level="win:Informational" keywords="HTTP_KEYWORD_CACHE" template="HTTPCacheTraceTaskAddedCacheEntryArgs"/>
// 
//     <template tid="HTTPCacheTraceTaskAddedCacheEntryArgs_V1">
//      <data name="Uri" inType="win:UnicodeString"/>      v0
//      <data name="StatusCode" inType="win:UInt16"/>      v0
//      <data name="Verb" inType="win:AnsiString"/>        v0
//      <data name="HeaderLength" inType="win:UInt32"/>    v0
//      <data name="ContentLength" inType="win:UInt32"/>   v0
//      <data name="ExpirationTime" inType="win:UInt64"/>  v0
//      <data name="Encoding" inType="win:AnsiString"/>    v1
//     </template>

#define EVENT_ID_HttpService_HTTPCacheTraceTaskAddedCacheEntry  25
#define EVENT_ID_HttpService_HTTPCacheTraceTaskFlushedCache     27
typedef struct _EVENT_PARAM_HttpService_HTTPCacheTraceTaskAddedCacheEntry
{
    uint8_t  uri[1];         // Unicode wide char zero terminating string    
//  uint16_t statusCode;
//  uint8_t  verb[1];        // ASCII wide char zero terminating string
//  uint32_t headerLength;
//  uint32_t contentLength;
//  uint64_t expirationTime;
} EVENT_PARAM_HttpService_HTTPCacheTraceTaskAddedCacheEntry;

// ------------------------------------------------------------------------------
//
//  <event value="34" symbol="HTTPSSLTraceTaskSslConnEvent" version="0" task="HTTPSSLTraceTask" opcode="SslConnEvent" level="win:Informational" keywords="HTTP_KEYWORD_CONNECTION HTTP_KEYWORD_SSL" template="HTTPConnectionTraceTaskConnCleanupArgs"/>
//
//  <template tid="HTTPConnectionTraceTaskConnCleanupArgs">
//      <data name="ConnectionObj" inType="win:Pointer"/>
//  </template>

#define EVENT_ID_HttpService_HTTPSSLTraceTaskSslConnEvent    34
typedef struct _EVENT_PARAM_HttpService_HTTPTraceTaskConnCleanup {
    uint64_t connectionObj;
} EVENT_PARAM_HttpService_HTTPTraceTaskConnCleanup;
#pragma pack(pop)

// Builtin EVENT_TRACE_FLAG_NETWORK_TCPIP {9a280ac0-c8e0-11d1-84e2-00c04fb998a2}
//     https://docs.microsoft.com/en-us/windows/win32/etw/nt-kernel-logger-constants
//     (parsing e.g. https://processhacker.sourceforge.io/doc/etwmon_8c_source.html)

// EVENT_TRACE_FLAG_NETWORK_TCPIP {bf3a50c5-a9c9-4988-a005-2df0b7c80f80}
//     https://docs.microsoft.com/en-us/windows/win32/etw/nt-kernel-logger-constants

// Microsoft-Windows-DNS-Client {1c95126e-7eea-49a9-a3fe-a378b03ddb4d}
//     https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-18990/Microsoft-Windows-DNS-Client.xml

#endif