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
 * - From V3 on, each version's fields live in their own vN member, so a field is
 *   always read as counters.vN.field, where N is the version that introduced it
 * - A new version appends one vN member, so existing accesses never change
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
#define DRIVER_COUNTERS_VERSION_2 2
#define DRIVER_COUNTERS_VERSION_3 3

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

/**
 * @brief V2 counter fields - injection-related crash attribution and boot recovery counters.
 */
#define DRIVER_COUNTERS_V2_FIELDS \
    LONG64 CrashesDuringInjection; \
    LONG64 CrashesPostInjection; \
    LONG64 BootRecoveryCrashBootsDetected; \
    LONG64 BootRecoveryDriverSelfDisabled; \
    LONG64 BootRecoveryStabilityTimerFired;

/**
 * @brief V3 counter fields - automatic crash-recovery exclusions, skip-list
 * skips by entry source, and binary signature policy checks. The per-source
 * skip counters sum to v1.ProcessesSkippedExcluded.
 * ProcessesSignaturePolicyUnknown counts processes whose policy could not be
 * determined; they are still injected. ProcessesSkippedShortName counts
 * processes skipped because their image path kept an unexpanded 8.3 short name.
 */
#define DRIVER_COUNTERS_V3_FIELDS \
    LONG64 ProcessesAutomaticallyAddedToSkipList; \
    LONG64 ProcessesSkippedDefault; \
    LONG64 ProcessesSkippedCustom; \
    LONG64 ProcessesSkippedCrashRecovery; \
    LONG64 ProcessesSkippedSignaturePolicy; \
    LONG64 ProcessesSignaturePolicyUnknown; \
    LONG64 ProcessesSkippedShortName;

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

/**
 * @brief Driver performance and diagnostic counters (Version 2).
 *
 * @details
 * Embeds the frozen V1 fields under a 'v1' namespace and appends the V2
 * fields directly. Access pattern: counters.v1.<field> for V1 fields,
 * counters.<field> for V2 fields.
 */
typedef struct _DRIVER_COUNTERS_V2 {
    DRIVER_COUNTERS_V1 v1;
    DRIVER_COUNTERS_V2_FIELDS
} DRIVER_COUNTERS_V2, *PDRIVER_COUNTERS_V2;

/**
 * @brief Driver performance and diagnostic counters (Version 3).
 *
 * @details
 * Preserves the exact frozen V2 binary prefix and places every version's fields
 * in its own member. Access pattern: counters.vN.<field>, where N is the version
 * that introduced the field.
 */
typedef struct _DRIVER_COUNTERS_V3 {
    DRIVER_COUNTERS_V1 v1;
    struct {
        DRIVER_COUNTERS_V2_FIELDS
    } v2;
    struct {
        DRIVER_COUNTERS_V3_FIELDS
    } v3;
} DRIVER_COUNTERS_V3, *PDRIVER_COUNTERS_V3;

/*
 * A future version appends one vN member holding its own field macro after the
 * existing members. Every released layout stays an exact binary prefix and
 * existing counters.vN.<field> accesses never change.
 */

#endif // DDINJECTOR_PUBLIC_H
