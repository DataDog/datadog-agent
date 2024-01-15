#pragma once

#ifndef _STDINT
// we're using an older SDK that doesn't have these types.  Define them for clarity
typedef unsigned long long  uint64_t;
typedef unsigned long       uint32_t;
typedef unsigned short      uint16_t;
#endif

// for usage when building with the tracer
#ifndef _MSC_VER_
typedef __int64 LONG64;
#endif

// this type doesn't seem to be defined anyway
typedef unsigned char       uint8_t;

// define a version signature so that the driver won't load out of date structures, etc.
#define DD_PROCMONDRIVER_VERSION       0x02
#define DD_PROCMONDRIVER_SIGNATURE     ((uint64_t)0xDD01 << 32 | DD_PROCMONDRIVER_VERSION)
#define DD_PROCMONDRIVER_DEVICE_TYPE   FILE_DEVICE_UNKNOWN
// for more information on defining control codes, see
// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/defining-i-o-control-codes
//
// Vendor codes start with 0x800



#define DDPROCMONDRIVER_IOCTL_START  CTL_CODE(DD_PROCMONDRIVER_DEVICE_TYPE, \
                                              0x801, \
                                              METHOD_OUT_DIRECT,\
                                              FILE_ANY_ACCESS)

#define DDPROCMONDRIVER_IOCTL_STOP  CTL_CODE(DD_PROCMONDRIVER_DEVICE_TYPE, \
                                              0x802, \
                                              METHOD_OUT_DIRECT,\
                                              FILE_ANY_ACCESS)

#define DDPROCMONDRIVER_IOCTL_GETSTATS  CTL_CODE(DD_PROCMONDRIVER_DEVICE_TYPE, \
                                              0x803, \
                                              METHOD_OUT_DIRECT,\
                                              FILE_ANY_ACCESS)

typedef enum _dd_notify_type {
    DD_NOTIFY_STOP = 0,
    DD_NOTIFY_START = 1,
} DD_NOTIFY_TYPE;

typedef struct _dd_procmon_stats {
    uint64_t            processStartCount;
    uint64_t            processStopCount;

    uint64_t            missedNotifications;
    uint64_t            allocationFailures;
    uint64_t            workItemFailures;

} DD_PROCMON_STATS;

typedef struct _dd_process_notification {
    uint64_t            size;  // total size of structure.
    uint64_t            ProcessId;
    uint64_t            NotifyType; // as type DD_NOTIFY_TYPE
    // all below here only valid when NotifyType == DD_NOTIFY_START
    uint64_t            ImageFileLen;
    uint64_t            ImageFileOffset;
    uint64_t            CommandLineLen;
    uint64_t            CommandLineOffset;
} DD_PROCESS_NOTIFICATION;