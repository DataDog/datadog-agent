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
// crosses the cgo boundary. Returns 0 on success, -1 on an IOKit failure, -2
// when buffer was too small and -3 when a service's array could not be
// rendered in full, which is never reported as a partial read. *written
// receives the byte count.
int dd_pkg_notableevents_read_pmu_boot_fault_info(
    char *buffer,
    size_t size,
    size_t *written);

#endif
