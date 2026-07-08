// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#define _GNU_SOURCE

#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/syscall.h>
#include <unistd.h>

#ifndef MFD_ALLOW_SEALING
#define MFD_ALLOW_SEALING 0x0002U
#endif

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

typedef void (*otel_initial_exec_publish_fn)(struct otel_thread_ctx_record *record);
typedef void (*otel_initial_exec_clear_fn)(void);

struct otel_initial_exec_fixture {
    void *handle;
    otel_initial_exec_publish_fn publish;
    otel_initial_exec_clear_fn clear;
};

static int load_initial_exec_fixture(struct otel_initial_exec_fixture *fixture) {
    memset(fixture, 0, sizeof(*fixture));

    char exe_path[PATH_MAX];
    ssize_t exe_len = readlink("/proc/self/exe", exe_path, sizeof(exe_path) - 1);
    if (exe_len < 0) {
        perror("readlink /proc/self/exe");
        return -1;
    }
    exe_path[exe_len] = '\0';

    char *slash = strrchr(exe_path, '/');
    if (slash == NULL) {
        fprintf(stderr, "cannot resolve tester directory from %s\n", exe_path);
        return -1;
    }
    slash[1] = '\0';

    char fixture_path[PATH_MAX];
    int written = snprintf(fixture_path, sizeof(fixture_path), "%slibotel_tls_initial_exec_fixture.so", exe_path);
    if (written < 0 || written >= (int)sizeof(fixture_path)) {
        fprintf(stderr, "fixture path is too long\n");
        return -1;
    }

    fixture->handle = dlopen(fixture_path, RTLD_NOW | RTLD_LOCAL);
    if (fixture->handle == NULL) {
        fprintf(stderr, "dlopen(%s): %s\n", fixture_path, dlerror());
        return -1;
    }

    dlerror();
    *(void **)(&fixture->publish) = dlsym(fixture->handle, "otel_initial_exec_publish");
    const char *err = dlerror();
    if (err != NULL) {
        fprintf(stderr, "dlsym(otel_initial_exec_publish): %s\n", err);
        dlclose(fixture->handle);
        return -1;
    }

    dlerror();
    *(void **)(&fixture->clear) = dlsym(fixture->handle, "otel_initial_exec_clear");
    err = dlerror();
    if (err != NULL) {
        fprintf(stderr, "dlsym(otel_initial_exec_clear): %s\n", err);
        dlclose(fixture->handle);
        return -1;
    }

    return 0;
}

static void unload_initial_exec_fixture(struct otel_initial_exec_fixture *fixture) {
    if (fixture->handle != NULL) {
        dlclose(fixture->handle);
    }
    memset(fixture, 0, sizeof(*fixture));
}

static __int128 atouint128(char *s) {
    if (s == NULL) {
        return 0;
    }

    __int128 val = 0;
    for (; *s != 0 && *s >= '0' && *s <= '9'; s++) {
        val = (10 * val) + (*s - '0');
    }
    return val;
}

static void u64_to_be_bytes(uint64_t val, uint8_t *out) {
    out[0] = (uint8_t)(val >> 56);
    out[1] = (uint8_t)(val >> 48);
    out[2] = (uint8_t)(val >> 40);
    out[3] = (uint8_t)(val >> 32);
    out[4] = (uint8_t)(val >> 24);
    out[5] = (uint8_t)(val >> 16);
    out[6] = (uint8_t)(val >> 8);
    out[7] = (uint8_t)(val);
}

static int create_tracer_memfd(void) {
    const char tracer_data[] =
        "\x86"
        "\xae"
        "schema_version"
        "\x02"
        "\xaf"
        "tracer_language"
        "\xa3"
        "cpp"
        "\xae"
        "tracer_version"
        "\xa5"
        "0.0.1"
        "\xa8"
        "hostname"
        "\xa4"
        "test"
        "\xac"
        "service_name"
        "\xa8"
        "oteltest"
        "\xba"
        "threadlocal_attribute_keys"
        "\x93"
        "\xab"
        "http.method"
        "\xab"
        "http.target"
        "\xa9"
        "http.user";

    int fd = syscall(SYS_memfd_create, "datadog-tracer-info-oteltest", MFD_ALLOW_SEALING);
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

static void fill_otel_record(struct otel_record_with_attrs *record, char *trace_arg, char *span_arg) {
    memset(record, 0, sizeof(*record));

    __int128 trace_id = atouint128(trace_arg);
    uint64_t span_id = (uint64_t)atol(span_arg);
    uint64_t trace_hi = (uint64_t)(trace_id >> 64);
    uint64_t trace_lo = (uint64_t)(trace_id);

    u64_to_be_bytes(trace_hi, &record->header.trace_id[0]);
    u64_to_be_bytes(trace_lo, &record->header.trace_id[8]);
    u64_to_be_bytes(span_id, record->header.span_id);

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

    record->header.attrs_data_size = off;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    record->header.valid = 1;
}

static int otel_span_open(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-open <trace_id> <span_id> <file_path>\n");
        return EXIT_FAILURE;
    }

    pthread_self();

    struct otel_initial_exec_fixture fixture;
    if (load_initial_exec_fixture(&fixture) < 0) {
        return EXIT_FAILURE;
    }

    int memfd = create_tracer_memfd();
    if (memfd < 0) {
        unload_initial_exec_fixture(&fixture);
        return EXIT_FAILURE;
    }

    usleep(500000);

    struct otel_record_with_attrs record;
    fill_otel_record(&record, argv[1], argv[2]);
    fixture.publish(&record.header);

    int fd = open(argv[3], O_CREAT, 0777);
    if (fd < 0) {
        perror("open");
        fixture.clear();
        close(memfd);
        unload_initial_exec_fixture(&fixture);
        return EXIT_FAILURE;
    }
    close(fd);
    unlink(argv[3]);

    fixture.clear();
    close(memfd);
    unload_initial_exec_fixture(&fixture);
    return EXIT_SUCCESS;
}

int main(int argc, char **argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    if (strcmp(argv[1], "check") == 0) {
        pthread_self();
        struct otel_initial_exec_fixture fixture;
        if (load_initial_exec_fixture(&fixture) < 0) {
            return EXIT_FAILURE;
        }
        fixture.clear();
        unload_initial_exec_fixture(&fixture);
        return EXIT_SUCCESS;
    }

    int sub_argc = argc - 1;
    char **sub_argv = argv + 1;

    if (strcmp(sub_argv[0], "otel-span-open") == 0) {
        return otel_span_open(sub_argc, sub_argv);
    }

    fprintf(stderr, "Unknown command: %s\n", sub_argv[0]);
    return EXIT_FAILURE;
}
