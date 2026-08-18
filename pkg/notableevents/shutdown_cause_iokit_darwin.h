// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#ifndef PKG_NOTABLEEVENTS_SHUTDOWN_CAUSE_IOKIT_DARWIN_H
#define PKG_NOTABLEEVENTS_SHUTDOWN_CAUSE_IOKIT_DARWIN_H

#include <stddef.h>

// dd_pkg_notableevents_read_pmu_boot_fault_info flattens every
// IOPMUBootFaultInfo string array in the IOService plane into buffer. Every
// token from every publishing service lands in the same flat, 0x1f-separated
// sequence with no service-boundary marker, so no Core Foundation type
// crosses the cgo boundary. A service whose array exceeds
// DD_PMU_MAX_TOKENS_PER_SERVICE, DD_PMU_MAX_TOKEN_CHARS or the buffer's
// remaining capacity is dropped and logged rather than failing the read, and
// a token already emitted by an earlier element or service is never written
// twice. Returns 0 on success and -1 on an IOKit failure or invalid
// arguments. *written receives the byte count.
int dd_pkg_notableevents_read_pmu_boot_fault_info(
    char *buffer,
    size_t size,
    size_t *written);

#endif
