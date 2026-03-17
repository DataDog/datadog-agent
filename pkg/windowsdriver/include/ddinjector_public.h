/**
 * @file ddinjector_public.h
 * @brief Public interface definitions for ddinjector driver.
 * @copyright Copyright 2025-present Datadog, Inc.
 *
 * @details
 * Defines the public IOCTL interface and counter structures for communication
 * between the ddinjector kernel driver and user-mode services. This header
 * is shared between kernel and user-mode components.
 *
 * VERSIONING POLICY:
 * - Counter structures follow strict versioning for backward compatibility
 * - V1 structures must NEVER be modified once released
 * - New versions use macro-based field definitions to avoid deep nesting
 * - Access pattern: counters.v1.field, counters.v2.field, counters.field (current)
 * - Clients query MaxSupportedCounterVersion and request specific versions
 *
 */

#ifndef DDINJECTOR_PUBLIC_H
#define DDINJECTOR_PUBLIC_H


//
// IOCTL Definitions
//
#define DDINJECTOR_DEVICE_TYPE 0x8000

#define IOCTL_GET_DRIVER_CAPABILITIES \
    (unsigned int )(CTL_CODE(DDINJECTOR_DEVICE_TYPE, 0x800, METHOD_BUFFERED, FILE_READ_DATA))

#define IOCTL_GET_COUNTERS \
    (unsigned int )(CTL_CODE(DDINJECTOR_DEVICE_TYPE, 0x801, METHOD_OUT_DIRECT, FILE_READ_DATA))

//
// Version Definitions
//
#define DRIVER_COUNTERS_VERSION_1 1

//
// Structure Definitions
//

/**
 * @brief Driver capabilities information.
 */
typedef struct _DRIVER_CAPABILITIES {
    ULONG MaxSupportedCounterVersion; // Highest counter version supported
    ULONG Reserved[3]; // Reserved for future use
} DRIVER_CAPABILITIES, *PDRIVER_CAPABILITIES;

/**
 * @brief Counter request structure for specifying desired version.
 */
typedef struct _COUNTER_REQUEST {
    ULONG RequestedVersion; // Version of counters to retrieve
} COUNTER_REQUEST, *PCOUNTER_REQUEST;

//
// Counter Field Definitions (Macro-based for extensibility)
//

/**
 * @brief V1 counter fields - Process management and injection performance.
 * @warning These fields must NEVER be modified once released.
 */
#define DRIVER_COUNTERS_V1_FIELDS \
    LONG64 ProcessesAddedToInjectionTracker; \
    LONG64 ProcessesRemovedFromInjectionTracker; \
    LONG64 ProcessesSkippedSubsystem; \
    LONG64 ProcessesSkippedContainer; \
    LONG64 ProcessesSkippedProtected; \
    LONG64 ProcessesSkippedSystem; \
    LONG64 ProcessesSkippedExcluded; \
    LONG64 InjectionAttempts; \
    LONG64 InjectionAttemptFailures; \
    LONG64 InjectionMaxTimeUs; \
    LONG64 InjectionSuccesses; \
    LONG64 InjectionFailures; \
    LONG64 PeCachingFailures; \
    LONG64 ImportDirectoryRestorationFailures; \
    LONG64 PeMemoryAllocationFailures; \
    LONG64 PeInjectionContextAllocated; \
    LONG64 PeInjectionContextCleanedup;

//
// Counter Structure Definitions
//

/**
 * @brief Driver performance and diagnostic counters (Version 1).
 * @warning This structure must NEVER be modified. Create V2 for new counters.
 *
 * @details
 * This is the base version of counters. All fields are directly accessible.
 * Future versions will nest this under a 'v1' namespace for clear versioning.
 */
typedef struct _DRIVER_COUNTERS_V1 {
    DRIVER_COUNTERS_V1_FIELDS
} DRIVER_COUNTERS_V1, *PDRIVER_COUNTERS_V1;

/*
 * FUTURE VERSION EXTENSION EXAMPLE:
 *
 * #define DRIVER_COUNTERS_VERSION_2 2
 *
 * #define DRIVER_COUNTERS_V2_FIELDS \
 *     LONG64 MemoryPoolAllocations; \
 *     LONG64 MemoryPoolFailures;
 *
 * typedef struct _DRIVER_COUNTERS_V2 {
 *     struct {
 *         DRIVER_COUNTERS_V1_FIELDS
 *     } v1;
 *     DRIVER_COUNTERS_V2_FIELDS
 * } DRIVER_COUNTERS_V2, *PDRIVER_COUNTERS_V2;
 *
 * Usage:
 *   counters.v1.ProcessesAddedToInjectionTracker  // V1 field
 *   counters.MemoryPoolAllocations                // V2 field
 */

#endif // DDINJECTOR_PUBLIC_H
