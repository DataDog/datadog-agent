// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#ifndef PKG_NOTABLEEVENTS_SHUTDOWN_CAUSE_IOKIT_DARWIN_H
#define PKG_NOTABLEEVENTS_SHUTDOWN_CAUSE_IOKIT_DARWIN_H

#include <stddef.h>

// DD_PMU_MAX_TOKENS_PER_SERVICE and DD_PMU_MAX_TOKEN_CHARS are sized from the
// largest known real payload (80-token dictionary, longest token 35 bytes)
// plus margin: 10% over token count, 50% over token length. A service
// exceeding DD_PMU_MAX_TOKENS_PER_SERVICE is dropped, not failed; a token
// exceeding DD_PMU_MAX_TOKEN_CHARS (content plus its trailing separator byte)
// is truncated rather than dropping its service. 
#define DD_PMU_MAX_TOKENS_PER_SERVICE 88
#define DD_PMU_MAX_TOKEN_CHARS 53

// dd_pkg_notableevents_read_pmu_boot_fault_info flattens every
// IOPMUBootFaultInfo array in the IOService plane into buffer, as one flat
// 0x1f-separated token sequence with no service-boundary marker, so no Core
// Foundation type crosses the cgo boundary. An oversized token is truncated;
// a service exceeding its bounds is dropped and logged rather than failing
// the read; a duplicate token is never written twice. Returns 0 on success,
// -1 on an IOKit failure or invalid arguments. *written receives the byte
// count.
int dd_pkg_notableevents_read_pmu_boot_fault_info(
    char *buffer,
    size_t size,
    size_t *written);

#endif
