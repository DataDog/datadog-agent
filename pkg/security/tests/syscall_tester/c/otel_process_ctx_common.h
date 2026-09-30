// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Publishes an OTel process context (OTEP 4719) the way libdatadog does, for the
// testers that stand in for an instrumented process: a memfd named OTEL_CTX
// holding the header, mapped private, the payload living outside it, and the
// naming call the agent watches for made last.

#ifndef OTEL_PROCESS_CTX_COMMON_H
#define OTEL_PROCESS_CTX_COMMON_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <time.h>
#include <unistd.h>

#ifndef PR_SET_VMA
#define PR_SET_VMA 0x53564d41
#endif
#ifndef PR_SET_VMA_ANON_NAME
#define PR_SET_VMA_ANON_NAME 0
#endif

struct otel_process_ctx_header {
    char signature[8];
    uint32_t version;
    uint32_t payload_size;
    uint64_t monotonic_published_at_ns;
    uint64_t payload_ptr;
};

// Protobuf field numbers of the sliver of ProcessContext published here; see
// pkg/security/otelprocessctx.
#define OTEL_PB_EXTRA_ATTRIBUTES 2
#define OTEL_PB_KV_KEY 1
#define OTEL_PB_KV_VALUE 2
#define OTEL_PB_ANY_STRING 1
#define OTEL_PB_ANY_INT 3
#define OTEL_PB_ANY_ARRAY 5
#define OTEL_PB_ARRAY_VALUES 1

#define OTEL_PB_WIRE_VARINT 0
#define OTEL_PB_WIRE_BYTES 2

static inline size_t otel_pb_varint(uint8_t *out, uint64_t value) {
    size_t off = 0;
    do {
        uint8_t byte = value & 0x7f;
        value >>= 7;
        if (value != 0) {
            byte |= 0x80;
        }
        out[off++] = byte;
    } while (value != 0);
    return off;
}

static inline size_t otel_pb_tag(uint8_t *out, int field, int wire) {
    return otel_pb_varint(out, ((uint64_t)field << 3) | (uint64_t)wire);
}

static inline size_t otel_pb_bytes(uint8_t *out, int field, const void *data, size_t len) {
    size_t off = otel_pb_tag(out, field, OTEL_PB_WIRE_BYTES);
    off += otel_pb_varint(out + off, len);
    memcpy(out + off, data, len);
    return off + len;
}

static inline size_t otel_pb_string_value(uint8_t *out, const char *value) {
    return otel_pb_bytes(out, OTEL_PB_ANY_STRING, value, strlen(value));
}

static inline size_t otel_pb_int_value(uint8_t *out, int64_t value) {
    size_t off = otel_pb_tag(out, OTEL_PB_ANY_INT, OTEL_PB_WIRE_VARINT);
    return off + otel_pb_varint(out + off, (uint64_t)value);
}

// Appends one KeyValue of the extra attributes, its value already encoded as an
// AnyValue in `value`.
static inline size_t otel_pb_attribute(uint8_t *out, const char *key, const uint8_t *value, size_t value_len) {
    uint8_t kv[512];
    size_t kv_len = otel_pb_bytes(kv, OTEL_PB_KV_KEY, key, strlen(key));
    kv_len += otel_pb_bytes(kv + kv_len, OTEL_PB_KV_VALUE, value, value_len);
    return otel_pb_bytes(out, OTEL_PB_EXTRA_ATTRIBUTES, kv, kv_len);
}

static inline size_t otel_pb_string_attribute(uint8_t *out, const char *key, const char *value) {
    uint8_t any[256];
    size_t any_len = otel_pb_string_value(any, value);
    return otel_pb_attribute(out, key, any, any_len);
}

static inline size_t otel_pb_int_attribute(uint8_t *out, const char *key, int64_t value) {
    uint8_t any[32];
    size_t any_len = otel_pb_int_value(any, value);
    return otel_pb_attribute(out, key, any, any_len);
}

static inline size_t otel_pb_string_array_attribute(uint8_t *out, const char *key, const char *const *values,
                                                    size_t count) {
    uint8_t array[512];
    size_t array_len = 0;
    for (size_t i = 0; i < count; i++) {
        uint8_t entry[256];
        size_t entry_len = otel_pb_string_value(entry, values[i]);
        array_len += otel_pb_bytes(array + array_len, OTEL_PB_ARRAY_VALUES, entry, entry_len);
    }

    uint8_t any[600];
    size_t any_len = otel_pb_bytes(any, OTEL_PB_ANY_ARRAY, array, array_len);
    return otel_pb_attribute(out, key, any, any_len);
}

// The published payload, which the header points at. Static because the header
// keeps pointing at it for the life of the process, exactly as the publisher's
// own buffer does.
static uint8_t otel_process_ctx_payload[1024];

// otel_process_ctx_publish publishes `payload` as the process context of this
// process. The naming call is made last and its outcome ignored -- the kernel
// rejects it on the file-backed mapping used here, and before 5.17 rejects the
// option outright -- because making it is what tells a reader to come and read.
static inline int otel_process_ctx_publish(const uint8_t *payload, size_t payload_size) {
    long page_size = sysconf(_SC_PAGESIZE);

    if (payload_size > sizeof(otel_process_ctx_payload)) {
        fprintf(stderr, "process context payload too large: %zu\n", payload_size);
        return -1;
    }
    memcpy(otel_process_ctx_payload, payload, payload_size);

    int fd = (int)syscall(SYS_memfd_create, "OTEL_CTX", 0);
    if (fd < 0) {
        perror("memfd_create OTEL_CTX");
        return -1;
    }
    if (ftruncate(fd, page_size) < 0) {
        perror("ftruncate OTEL_CTX");
        close(fd);
        return -1;
    }

    void *mapping = mmap(NULL, page_size, PROT_READ | PROT_WRITE, MAP_PRIVATE, fd, 0);
    close(fd);
    if (mapping == MAP_FAILED) {
        perror("mmap OTEL_CTX");
        return -1;
    }

    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);

    struct otel_process_ctx_header *header = (struct otel_process_ctx_header *)mapping;
    memcpy(header->signature, "OTEL_CTX", sizeof(header->signature));
    header->version = 2;
    header->payload_size = (uint32_t)payload_size;
    header->payload_ptr = (uint64_t)(uintptr_t)otel_process_ctx_payload;
    // Published last: until this is non-zero a reader must consider the header
    // unfinished and come back later.
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    header->monotonic_published_at_ns = (uint64_t)now.tv_sec * 1000000000ull + (uint64_t)now.tv_nsec;

    prctl(PR_SET_VMA, PR_SET_VMA_ANON_NAME, (unsigned long)mapping, (unsigned long)page_size, (unsigned long)"OTEL_CTX");
    return 0;
}

#endif
