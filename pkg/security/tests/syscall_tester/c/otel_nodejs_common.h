// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Helpers shared by the otel-node-span-* testers, which stand in for V8 rather
// than run Node: what the reader walks is a handful of offsets into plain memory,
// so the object graph below reproduces it exactly, and can produce on demand the
// shapes a real Node process would produce only rarely -- a bucket collision, a
// chain deeper than the reader follows.

#ifndef OTEL_NODEJS_COMMON_H
#define OTEL_NODEJS_COMMON_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <time.h>
#include <unistd.h>

#include "otel_process_ctx_common.h"
#include "otel_tls_common.h"

// for the tracer metadata memfd below
#ifndef MFD_ALLOW_SEALING
#define MFD_ALLOW_SEALING 0x0002U
#endif

// --- V8 layout, as the writer publishes it in its process context ---

#define OTEL_NODEJS_TAGGED_SIZE 8
#define OTEL_NODEJS_JS_MAP_TABLE_OFFSET 0x18
#define OTEL_NODEJS_OHM_HEADER_SIZE 0x10
#define OTEL_NODEJS_WRAPPED_OBJECT_OFFSET 24
#define OTEL_NODEJS_NATIVE_WRAP_FIELDS_OFFSET 0

// A pointer to a heap object carries the low bit; a Smi does not, its payload
// sitting in the top half of the word.
#define OTEL_NODEJS_TAG(ptr) ((uint64_t)(uintptr_t)(ptr) | 1)
#define OTEL_NODEJS_SMI(value) ((uint64_t)((int64_t)(value)) << 32)

// kept in sync with OTEL_NODEJS_MAX_CHAIN in helpers/span_nodejs.h
#define OTEL_NODEJS_MAX_CHAIN 4

// four buckets being what V8 gives a map holding a handful of entries
#define OTEL_NODEJS_BUCKETS 4
#define OTEL_NODEJS_CAPACITY (OTEL_NODEJS_BUCKETS * 2)
#define OTEL_NODEJS_EMPTY_BUCKET (-1)

// Byte-compatible with struct otel_nodejs_ctx_t in
// pkg/security/ebpf/c/include/structs/span_context.h.
struct otel_thread_ctx_nodejs {
    uint64_t cped_slot;
    uint64_t als_handle;
    int32_t als_identity_hash;
    int32_t _pad;
    uint64_t undefined_addr;
};

// Enough of a JSObject for the two the reader walks through: it reads the table
// of the AsyncContextFrame and the native pointer of the record's wrapper, both
// of which land at 0x18.
struct otel_nodejs_js_object {
    uint64_t map;
    uint64_t properties;
    uint64_t elements;
    uint64_t slot; // table (tagged) for a Map, native pointer for a wrapper
};

// The backing OrderedHashMap of a JS Map: counts, then one entry index per
// bucket, then the entries, three words each.
struct otel_nodejs_ordered_hash_map {
    uint64_t header[OTEL_NODEJS_OHM_HEADER_SIZE / 8];
    uint64_t num_elements;
    uint64_t num_deleted;
    uint64_t num_buckets;
    uint64_t buckets[OTEL_NODEJS_BUCKETS];
    uint64_t entries[OTEL_NODEJS_CAPACITY * 3];
};

// The C++ object wrapping a record, which the JS wrapper points at.
struct otel_nodejs_ctx_wrap {
    struct otel_thread_ctx_record *record;
};

// The whole graph, allocated in one go.
struct otel_nodejs_graph {
    struct otel_thread_ctx_nodejs discovery;

    uint64_t cped;         // the isolate slot V8 swaps as continuations change
    uint64_t als_slot;     // the handle the collector keeps pointing at the ALS
    uint64_t als_object;   // the AsyncLocalStorage instance itself
    uint64_t undefined;    // this isolate's undefined singleton

    struct otel_nodejs_js_object frame;
    struct otel_nodejs_js_object wrapper;
    struct otel_nodejs_ordered_hash_map table;
    struct otel_nodejs_ctx_wrap wrap;
    struct otel_record_with_attrs record;
};

// How the entry the reader is after sits in the bucket its key hashes to.
enum otel_nodejs_entry_placement {
    otel_nodejs_entry_head,     // first in the chain, as it is in a real frame
    otel_nodejs_entry_chained,  // behind another key that hashed to the bucket
    otel_nodejs_entry_deep,     // behind more of them than the reader follows
    otel_nodejs_entry_absent,   // this frame holds no context at all
};

// The identity hash of the writer's AsyncLocalStorage instance, which picks the
// bucket. Any value does; this one is not a multiple of the bucket count, so a
// wrong mask lands in the wrong bucket rather than accidentally the right one.
#define OTEL_NODEJS_ALS_HASH 0x5eed0003

// otel_nodejs_build lays out the object graph a reader walks, with the entry it
// is after placed as asked.
static inline void otel_nodejs_build(struct otel_nodejs_graph *g, enum otel_nodejs_entry_placement placement) {
    memset(g, 0, sizeof(*g));

    // a tagged address of its own is all the reader needs of undefined
    g->undefined = OTEL_NODEJS_TAG(&g->undefined);

    g->als_object = OTEL_NODEJS_TAG(&g->als_object);
    g->als_slot = g->als_object;

    g->wrap.record = &g->record.header;
    g->wrapper.slot = (uint64_t)(uintptr_t)&g->wrap;

    g->table.num_elements = OTEL_NODEJS_SMI(1);
    g->table.num_deleted = OTEL_NODEJS_SMI(0);
    g->table.num_buckets = OTEL_NODEJS_SMI(OTEL_NODEJS_BUCKETS);
    for (int i = 0; i < OTEL_NODEJS_BUCKETS; i++) {
        g->table.buckets[i] = OTEL_NODEJS_SMI(OTEL_NODEJS_EMPTY_BUCKET);
    }
    for (int i = 0; i < OTEL_NODEJS_CAPACITY * 3; i++) {
        g->table.entries[i] = OTEL_NODEJS_SMI(OTEL_NODEJS_EMPTY_BUCKET);
    }

    int bucket = OTEL_NODEJS_ALS_HASH & (OTEL_NODEJS_BUCKETS - 1);

    // How many other keys hashed to the same bucket. V8 puts the newest entry at
    // the head of the chain, so inserting ours first is what puts them in front.
    int decoys = 0;
    if (placement == otel_nodejs_entry_chained) {
        decoys = 1;
    } else if (placement == otel_nodejs_entry_deep) {
        decoys = OTEL_NODEJS_MAX_CHAIN;
    }

    int entry = 0;
    int head = OTEL_NODEJS_EMPTY_BUCKET;
    if (placement != otel_nodejs_entry_absent) {
        g->table.entries[entry * 3] = g->als_object;
        g->table.entries[entry * 3 + 1] = OTEL_NODEJS_TAG(&g->wrapper);
        g->table.entries[entry * 3 + 2] = OTEL_NODEJS_SMI(head);
        head = entry;
        entry++;
    }

    for (int i = 0; i < decoys && entry < OTEL_NODEJS_CAPACITY; i++, entry++) {
        // a key of its own, which the reader must walk past
        g->table.entries[entry * 3] = OTEL_NODEJS_TAG(&g->table.entries[entry * 3]);
        g->table.entries[entry * 3 + 1] = g->undefined;
        g->table.entries[entry * 3 + 2] = OTEL_NODEJS_SMI(head);
        head = entry;
    }
    g->table.buckets[bucket] = OTEL_NODEJS_SMI(head);

    g->frame.slot = OTEL_NODEJS_TAG(&g->table);
    g->cped = OTEL_NODEJS_TAG(&g->frame);

    g->discovery.cped_slot = (uint64_t)(uintptr_t)&g->cped;
    g->discovery.als_handle = (uint64_t)(uintptr_t)&g->als_slot;
    g->discovery.als_identity_hash = OTEL_NODEJS_ALS_HASH;
    g->discovery.undefined_addr = g->undefined;
}

// otel_nodejs_clear_context leaves the frame holding undefined, the way the
// writer does when a span ends and nothing takes its place.
static inline void otel_nodejs_clear_context(struct otel_nodejs_graph *g) {
    g->cped = g->undefined;
}

// The key names the indexes of a record's attributes select.
static const char *const otel_nodejs_attribute_keys[] = {
    "http.method",
    "http.target",
    "http.user",
};

// otel_nodejs_encode_process_ctx writes the process context payload of a Node.js
// writer: the schema its records follow, the key names their attributes select,
// and the V8 layout the reader walks them with.
static inline size_t otel_nodejs_encode_process_ctx(uint8_t *out) {
    size_t off = otel_pb_string_attribute(out, "threadlocal.schema_version", "nodejs_v1_dev");
    off += otel_pb_string_array_attribute(out + off, "threadlocal.attribute_key_map", otel_nodejs_attribute_keys,
                                          sizeof(otel_nodejs_attribute_keys) / sizeof(otel_nodejs_attribute_keys[0]));
    off += otel_pb_int_attribute(out + off, "threadlocal.tagged_size", OTEL_NODEJS_TAGGED_SIZE);
    off += otel_pb_int_attribute(out + off, "threadlocal.js_map_table_offset", OTEL_NODEJS_JS_MAP_TABLE_OFFSET);
    off += otel_pb_int_attribute(out + off, "threadlocal.ordered_hash_map_header_size", OTEL_NODEJS_OHM_HEADER_SIZE);
    off += otel_pb_int_attribute(out + off, "threadlocal.wrapped_object_offset", OTEL_NODEJS_WRAPPED_OBJECT_OFFSET);
    off += otel_pb_int_attribute(out + off, "threadlocal.native_wrap_fields_offset",
                                 OTEL_NODEJS_NATIVE_WRAP_FIELDS_OFFSET);
    return off;
}

// otel_nodejs_publish_process_ctx publishes what a Node.js writer publishes.
static inline int otel_nodejs_publish_process_ctx(void) {
    uint8_t payload[1024];
    return otel_process_ctx_publish(payload, otel_nodejs_encode_process_ctx(payload));
}

// The tracer metadata of a Node.js process. It carries no attribute key list: the
// process context above does, and this is what proves the reader takes it there.
static inline int otel_nodejs_create_tracer_memfd(void) {
    const char tracer_data[] =
        "\x85"
        "\xae" "schema_version" "\x02"
        "\xaf" "tracer_language" "\xa6" "nodejs"
        "\xae" "tracer_version" "\xa5" "0.0.1"
        "\xa8" "hostname" "\xa4" "test"
        "\xac" "service_name" "\xa8" "nodetest";

    int fd = (int)syscall(SYS_memfd_create, "datadog-tracer-info-nodetest", MFD_ALLOW_SEALING);
    if (fd < 0) {
        perror("memfd_create");
        return -1;
    }

    ssize_t written = write(fd, tracer_data, sizeof(tracer_data) - 1);
    if (written != (ssize_t)(sizeof(tracer_data) - 1)) {
        perror("memfd write");
        close(fd);
        return -1;
    }

    if (fcntl(fd, F_ADD_SEALS, F_SEAL_WRITE | F_SEAL_SHRINK | F_SEAL_GROW) < 0) {
        perror("memfd seal");
        close(fd);
        return -1;
    }

    return fd;
}

#endif
