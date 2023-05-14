#ifndef __ETW_H
#define __ETW_H

#include <inttypes.h>

// Flags
#define DD_ETW_TRACE_FLAG_DEFAULT      0
#define DD_ETW_TRACE_FLAG_ASYNC_EVENTS 0x00000001

// Bitmask
// Microsoft-Windows-HttpService  {dd5ef90a-6398-47a4-ad34-4dcecdef795f}
//     https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-18990/Microsoft-Windows-HttpService.xml
#define DD_ETW_TRACE_PROVIDER_HttpService  0x00000001

// Builtin EVENT_TRACE_FLAG_NETWORK_TCPIP {9a280ac0-c8e0-11d1-84e2-00c04fb998a2}
//     https://docs.microsoft.com/en-us/windows/win32/etw/nt-kernel-logger-constants
//     (parsing e.g. https://processhacker.sourceforge.io/doc/etwmon_8c_source.html)
#define DD_ETW_TRACE_PROVIDER_TCPIP        0x00000002

// EVENT_TRACE_FLAG_NETWORK_TCPIP {bf3a50c5-a9c9-4988-a005-2df0b7c80f80}
//     https://docs.microsoft.com/en-us/windows/win32/etw/nt-kernel-logger-constants
#define DD_ETW_TRACE_PROVIDER_UDP          0x00000004

// Microsoft-Windows-DNS-Client {1c95126e-7eea-49a9-a3fe-a378b03ddb4d}
//     https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-18990/Microsoft-Windows-DNS-Client.xml
#define DD_ETW_TRACE_PROVIDER_DNS          0x00000008

// Simplified and flattent bits and pices from EVENT_RECORD
//  Details are here https://docs.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_record
//
// 0> WinbDbg's dt /r _EVENT_RECORD
//
// ntdll!_EVENT_RECORD
//    +0x000 EventHeader      : _EVENT_HEADER
//       +0x000 Size             : Uint2B
//       +0x002 HeaderType       : Uint2B
//       +0x004 Flags            : Uint2B
//       +0x006 EventProperty    : Uint2B
//       +0x008 ThreadId         : Uint4B
//       +0x00c ProcessId        : Uint4B
//       +0x010 TimeStamp        : _LARGE_INTEGER
//          +0x000 LowPart          : Uint4B
//          +0x004 HighPart         : Int4B
//          +0x000 u                : <anonymous-tag>
//          +0x000 QuadPart         : Int8B
//       +0x018 ProviderId       : _GUID
//          +0x000 Data1            : Uint4B
//          +0x004 Data2            : Uint2B
//          +0x006 Data3            : Uint2B
//          +0x008 Data4            : [8] UChar
//       +0x028 EventDescriptor  : _EVENT_DESCRIPTOR
//          +0x000 Id               : Uint2B
//          +0x002 Version          : UChar
//          +0x003 Channel          : UChar
//          +0x004 Level            : UChar
//          +0x005 Opcode           : UChar
//          +0x006 Task             : Uint2B
//          +0x008 Keyword          : Uint8B
//       +0x038 KernelTime       : Uint4B
//       +0x03c UserTime         : Uint4B
//       +0x038 ProcessorTime    : Uint8B
//       +0x040 ActivityId       : _GUID
//          +0x000 Data1            : Uint4B
//          +0x004 Data2            : Uint2B
//          +0x006 Data3            : Uint2B
//          +0x008 Data4            : [8] UChar
//    +0x050 BufferContext    : _ETW_BUFFER_CONTEXT
//       +0x000 ProcessorNumber  : UChar
//       +0x001 Alignment        : UChar
//       +0x000 ProcessorIndex   : Uint2B
//       +0x002 LoggerId         : Uint2B
//    +0x054 ExtendedDataCount : Uint2B
//    +0x056 UserDataLength   : Uint2B
//    +0x058 ExtendedData     : Ptr64 _EVENT_HEADER_EXTENDED_DATA_ITEM
//       +0x000 Reserved1        : Uint2B
//       +0x002 ExtType          : Uint2B
//       +0x004 Linkage          : Pos 0, 1 Bit
//       +0x004 Reserved2        : Pos 1, 15 Bits
//       +0x006 DataSize         : Uint2B
//       +0x008 DataPtr          : Uint8B
//    +0x060 UserData         : Ptr64 Void
//    +0x068 UserContext      : Ptr64 Void

#pragma pack(push)
#pragma pack(1)

// Cloned from <guiddef.h> to simplify CGO
typedef struct _DDGUID {
    unsigned long  Data1;
    unsigned short Data2;
    unsigned short Data3;
    unsigned char  Data4[8];
} DDGUID;

typedef struct _DD_ETW_EVENT
{
    char     pad1[0xc];      // +0x00
    uint32_t pid;            // +0x0c
    uint64_t timeStamp;      // +0x10
    DDGUID   providerId;     // +0x18
    uint16_t id;             // +0x28
    uint8_t  version;        // +0x2A
    uint8_t  channel;        // +0x2B
    uint8_t  level;          // +0x2C
    uint8_t  opcode;         // +0x2D
    uint16_t task;           // +0x2E
    uint64_t keyword;        // +0x30
    uint8_t  pad2[0x8];      // +0x38
    DDGUID   activityId;     // +0x40
    uint8_t  pad3[6];        // +0x38
    uint16_t userDataLength; // +0x56
    uint8_t  pad4[8];        // +0x38
    uint8_t* userData;       // +0x60
} DD_ETW_EVENT;

typedef struct _DD_ETW_EVENT_INFO
{
    DD_ETW_EVENT* event;
    uint64_t      provider;
    DDGUID*       relatedActivityId;
} DD_ETW_EVENT_INFO;

#pragma pack(pop)

typedef void (__stdcall* ETW_EVENT_CALLBACK)(DD_ETW_EVENT_INFO* eventInfo);

#define SUBSCIPTION_NAME_MAX_LEN 128

// Calls
//  * ControlTrace (to stop old global subscription in case of crash)
//  * StartTrace
//  * EnableTraceEx2
//  * OpenTrace
//  * ProcessTrace (It is blocking call - events recived on this thread)
//
// providers - OR-ed DD_ETW_TRACE_PROVIDER_XXX flags
// flags     - OR-ed DD_ETW_TRACE_FLAG_XXX flags
int StartEtwSubscription(const char* subscriptionName, int64_t providers, int64_t flags, ETW_EVENT_CALLBACK callback);

// Calls
//  * ControlTrace
void StopEtwSubscription();

#endif
