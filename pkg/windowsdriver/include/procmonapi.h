#pragma once

#ifndef _STDINT

// Define common types for older SDKs.
typedef unsigned long long uint64_t;

typedef unsigned long uint32_t;

typedef unsigned short uint16_t;

#endif

// For usage when building with the tracer
#ifndef _MSC_VER_
typedef __int64 LONG64;
#endif

// This type doesn't seem to be defined anyway
typedef unsigned char uint8_t;

// Define a version signature so that the driver won't load out of date structures, etc.
#define DD_PROCMONDRIVER_VERSION 0x05

#define DD_PROCMONDRIVER_DEVICE_TYPE FILE_DEVICE_UNKNOWN

/**
  * @brief The procmon driver payload signature.
  */
#define DD_PROCMONDRIVER_SIGNATURE ((uint64_t)0xDD01 << 32 | DD_PROCMONDRIVER_VERSION)

// For more information on defining control codes, see
// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/defining-i-o-control-codes
//
// Vendor codes start with 0x800

/**
  * @brief I/O control code for getting ddprocmon global stats.
  */
#define DD_PROCMONDRIVER_IOCTL_START CTL_CODE(DD_PROCMONDRIVER_DEVICE_TYPE, \
    0x801,                                                                  \
    METHOD_OUT_DIRECT,                                                      \
    FILE_ANY_ACCESS)

/**
  * @brief I/O control code for stop monitoring.
  */
#define DD_PROCMONDRIVER_IOCTL_STOP CTL_CODE(DD_PROCMONDRIVER_DEVICE_TYPE, \
    0x802,                                                                 \
    METHOD_OUT_DIRECT,                                                     \
    FILE_ANY_ACCESS)

/**
  * @brief I/O control code for getting ddprocmon global stats.
  */
#define DD_PROCMONDRIVER_IOCTL_GETSTATS CTL_CODE(DD_PROCMONDRIVER_DEVICE_TYPE, \
    0x803,                                                                     \
    METHOD_OUT_DIRECT,                                                         \
    FILE_ANY_ACCESS)

/**
  * @brief Process notification type.
  */
typedef enum _DD_NOTIFY_TYPE {
    DD_NOTIFY_STOP = 0,
    DD_NOTIFY_START = 1,

} DD_NOTIFY_TYPE;

/**
  * @brief Process monitor global statistics.
  */
typedef struct _DD_PROCMON_STATS {
    /**
      * @brief Total count of process starts detected.
      */
    uint64_t ProcessStartCount;

    /**
      * @brief Total count of process stops detected.
      */
    uint64_t ProcessStopCount;

    /**
      * @brief Total count of notifications that missed processing.
      */
    uint64_t MissedNotifications;

    /**
      * @brief Total count of failed allocations for the queue.
      */
    uint64_t AllocationFailures;

    /**
      * @brief Total count of failed work item allocations.
      */
    uint64_t WorkItemFailures;

    /**
      * @brief Total number of times when the user-mode destination buffer was
      *        insufficient for the notification data.
      */
    uint64_t ReadBufferToSmallErrors;

} DD_PROCMON_STATS;

/**
  * @brief Process notification data.
  */
typedef struct _DD_PROCESS_NOTIFICATION {
    /**
      * @brief Total size of the structure.
      */
    uint64_t Size;

    /**
      * @brief Total size required to get structure.
      */
    uint64_t SizeNeeded;

    /**
      * @brief PID.
      */
    uint64_t ProcessId;

    /**
      * @brief Values defined by DD_NOTIFY_TYPE.
      */
    uint64_t NotifyType;

    // All below here only valid when NotifyType == DD_NOTIFY_START

    /**
      * @brief Parent PID.
      */
    uint64_t ParentProcessId;

    /**
      * @brief PID that created this process.
      */
    uint64_t CreatingProcessId;

    /**
      * @brief TID that created this process.
      */
    uint64_t CreatingThreadId;

    /**
      * @brief Length of the image file name.
      */
    uint64_t ImageFileLen;

    /**
      * @brief Offset where the image file name is located relative to this struct.
      */
    uint64_t ImageFileOffset;

    /**
      * @brief Length of the command line for the process.
      */
    uint64_t CommandLineLen;

    /**
      * @brief Offset where the command line is located relative to this struct.
      */
    uint64_t CommandLineOffset;

    /**
      * @brief Length of the process SID string.
      */
    uint64_t SidLen;

    /**
      * @brief Offset where the process SID string is located relative to this struct.
      */
    uint64_t SidOffset;

    /**
      * @brief Length of the memory block with the environment variables.
      */
    uint64_t EnvBlockLen;

    /**
      * @brief Offset where the block with the environment variables is located
      *       relative to this struct.
      */
    uint64_t EnvOffset;

} DD_PROCESS_NOTIFICATION;