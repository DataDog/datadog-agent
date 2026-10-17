// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Helpers shared by the otel-span-* testers, which publish an OTel thread local
// context record and then trigger a syscall for the eBPF reader to observe. Each
// tester defines otel_thread_ctx_v1 itself, never static, so the symbol lands in
// a symbol table the agent's resolver can read.
//
// Everything these testers publish about themselves goes in their OTel process
// context: a writer of thread contexts need not be a Datadog tracer, and this
// one is not one. The tracer metadata memfd a tracer would also seal is covered
// by the ddtracego testers instead.

#ifndef OTEL_TLS_COMMON_H
#define OTEL_TLS_COMMON_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "otel_process_ctx_common.h"

// The memfd seals reached glibc's fcntl.h in 2.25, and these testers are built
// against a 2.23 sysroot so they can run on every KMT host and inside the
// ubuntu:20.04 image the docker leg uses. The kernel ABI values are fixed.
#ifndef F_ADD_SEALS
#define F_ADD_SEALS (1024 + 9)
#endif
#ifndef F_SEAL_SHRINK
#define F_SEAL_SHRINK 0x0002
#endif
#ifndef F_SEAL_GROW
#define F_SEAL_GROW 0x0004
#endif
#ifndef F_SEAL_WRITE
#define F_SEAL_WRITE 0x0008
#endif

// Byte-compatible with struct otel_thread_ctx_record_t in
// pkg/security/ebpf/c/include/structs/span_context.h, which documents the
// layout; the eBPF reader parses these bytes out of the tester's memory.
struct otel_thread_ctx_record {
    uint8_t trace_id[16];
    uint8_t span_id[8];
    uint8_t valid;
    uint8_t _reserved;
    uint16_t attrs_data_size;
};

struct otel_record_with_attrs {
    struct otel_thread_ctx_record header;
    uint8_t attrs_data[64];
};

static inline __int128 otel_atouint128(char *s) {
    if (s == NULL) {
        return 0;
    }

    __int128 val = 0;
    for (; *s != 0 && *s >= '0' && *s <= '9'; s++) {
        val = (10 * val) + (*s - '0');
    }
    return val;
}

static inline void otel_u64_to_be_bytes(uint64_t val, uint8_t *out) {
    out[0] = (uint8_t)(val >> 56);
    out[1] = (uint8_t)(val >> 48);
    out[2] = (uint8_t)(val >> 40);
    out[3] = (uint8_t)(val >> 32);
    out[4] = (uint8_t)(val >> 24);
    out[5] = (uint8_t)(val >> 16);
    out[6] = (uint8_t)(val >> 8);
    out[7] = (uint8_t)(val);
}

// The names the key indexes of the records below select, in the order they are
// indexed by.
static const char *const otel_attribute_keys[] = {"http.method", "http.target", "http.user"};

// Publishes the process context of a writer using the plain thread-local schema.
// It is what makes the agent resolve this process's otel_thread_ctx_v1 TLS
// symbol, and what names the attributes of the records it publishes.
static inline int otel_publish_process_ctx(void) {
    uint8_t payload[512];
    size_t payload_size = otel_pb_string_attribute(payload, "threadlocal.schema_version", "tlsdesc_v1_dev");
    payload_size += otel_pb_string_array_attribute(payload + payload_size, "threadlocal.attribute_key_map",
                                                   otel_attribute_keys,
                                                   sizeof(otel_attribute_keys) / sizeof(otel_attribute_keys[0]));
    return otel_process_ctx_publish(payload, payload_size);
}

static inline void otel_fill_record(struct otel_record_with_attrs *record, char *trace_arg, char *span_arg) {
    memset(record, 0, sizeof(*record));

    __int128 trace_id = otel_atouint128(trace_arg);
    uint64_t span_id = (uint64_t)atol(span_arg);
    uint64_t trace_hi = (uint64_t)(trace_id >> 64);
    uint64_t trace_lo = (uint64_t)(trace_id);

    otel_u64_to_be_bytes(trace_hi, &record->header.trace_id[0]);
    otel_u64_to_be_bytes(trace_lo, &record->header.trace_id[8]);
    otel_u64_to_be_bytes(span_id, record->header.span_id);

    // A key index selects an entry of the attribute key map published in the
    // process context above.
    uint8_t *p = record->attrs_data;
    int off = 0;

    p[off++] = 0;
    p[off++] = 3;
    memcpy(&p[off], "GET", 3);
    off += 3;

    p[off++] = 1;
    p[off++] = 5;
    memcpy(&p[off], "/test", 5);
    off += 5;

    p[off++] = 2;
    p[off++] = 18;
    memcpy(&p[off], "will@datadoghq.com", 18);
    off += 18;

    // The signal fences are compiler barriers: they stop the stores that make a
    // record observable (valid here, the TLS pointer in the testers) from being
    // hoisted above the fill, so a reader never sees a half-built record.
    record->header.attrs_data_size = off;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    record->header.valid = 1;
}

#endif
