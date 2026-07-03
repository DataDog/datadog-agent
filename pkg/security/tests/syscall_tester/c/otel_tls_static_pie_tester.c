// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#define _GNU_SOURCE

#include <fcntl.h>
#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/syscall.h>
#include <sys/wait.h>
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

__attribute__((visibility("default"))) __thread struct otel_thread_ctx_record *otel_thread_ctx_v1 = NULL;

struct otel_record_with_attrs {
    struct otel_thread_ctx_record header;
    uint8_t attrs_data[64];
};

struct otel_thread_opts {
    char **argv;
    int memfd;
};

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

static int create_tracer_memfd() {
    const char tracer_data[] =
        "\x86"
        "\xae" "schema_version" "\x02"
        "\xaf" "tracer_language" "\xa3" "cpp"
        "\xae" "tracer_version" "\xa5" "0.0.1"
        "\xa8" "hostname" "\xa4" "test"
        "\xac" "service_name" "\xa8" "oteltest"
        "\xba" "threadlocal_attribute_keys"
        "\x93"
        "\xab" "http.method"
        "\xab" "http.target"
        "\xa9" "http.user";

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

static void publish_otel_record(struct otel_record_with_attrs *record) {
    otel_thread_ctx_v1 = NULL;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    otel_thread_ctx_v1 = &record->header;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}

static int prepare_otel_context(struct otel_record_with_attrs *record, char **argv, int *memfd) {
    *memfd = create_tracer_memfd();
    if (*memfd < 0) {
        return -1;
    }

    usleep(500000);
    fill_otel_record(record, argv[1], argv[2]);
    publish_otel_record(record);
    return 0;
}

static void *thread_otel_open(void *data) {
    struct otel_thread_opts *opts = (struct otel_thread_opts *)data;
    struct otel_record_with_attrs record;

    if (prepare_otel_context(&record, opts->argv, &opts->memfd) < 0) {
        return NULL;
    }

    int fd = open(opts->argv[3], O_CREAT, 0777);
    if (fd < 0) {
        perror("open");
        otel_thread_ctx_v1 = NULL;
        return NULL;
    }
    close(fd);
    unlink(opts->argv[3]);

    otel_thread_ctx_v1 = NULL;
    return NULL;
}

int otel_span_open(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-open <trace_id> <span_id> <file_path>\n");
        return EXIT_FAILURE;
    }

    struct otel_thread_opts opts = {.argv = argv, .memfd = -1};
    pthread_t thread;

    if (pthread_create(&thread, NULL, thread_otel_open, &opts) != 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    if (opts.memfd >= 0) {
        close(opts.memfd);
    }
    return EXIT_SUCCESS;
}

static void *thread_otel_exec(void *data) {
    struct otel_thread_opts *opts = (struct otel_thread_opts *)data;
    struct otel_record_with_attrs record;

    if (prepare_otel_context(&record, opts->argv, &opts->memfd) < 0) {
        return NULL;
    }

    execv(opts->argv[3], opts->argv + 3);
    perror("execv");
    otel_thread_ctx_v1 = NULL;
    return NULL;
}

int otel_span_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-exec <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }

    struct otel_thread_opts opts = {.argv = argv, .memfd = -1};
    pthread_t thread;

    if (pthread_create(&thread, NULL, thread_otel_exec, &opts) != 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    if (opts.memfd >= 0) {
        close(opts.memfd);
    }
    return EXIT_SUCCESS;
}

int otel_span_fork_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-fork-exec <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }

    int memfd = -1;
    struct otel_record_with_attrs record;
    if (prepare_otel_context(&record, argv, &memfd) < 0) {
        return EXIT_FAILURE;
    }

    pid_t child = fork();
    if (child < 0) {
        perror("fork");
        otel_thread_ctx_v1 = NULL;
        close(memfd);
        return EXIT_FAILURE;
    }
    if (child == 0) {
        execv(argv[3], argv + 3);
        perror("execv");
        _exit(EXIT_FAILURE);
    }

    int status;
    waitpid(child, &status, 0);

    otel_thread_ctx_v1 = NULL;
    close(memfd);
    return WIFEXITED(status) ? WEXITSTATUS(status) : EXIT_FAILURE;
}

int main(int argc, char **argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    if (strcmp(argv[1], "check") == 0) {
        return EXIT_SUCCESS;
    }

    int sub_argc = argc - 1;
    char **sub_argv = argv + 1;
    const char *cmd = sub_argv[0];

    if (strcmp(cmd, "otel-span-open") == 0) {
        return otel_span_open(sub_argc, sub_argv);
    }
    if (strcmp(cmd, "otel-span-exec") == 0) {
        return otel_span_exec(sub_argc, sub_argv);
    }
    if (strcmp(cmd, "otel-span-fork-exec") == 0) {
        return otel_span_fork_exec(sub_argc, sub_argv);
    }

    fprintf(stderr, "Unknown command: %s\n", cmd);
    return EXIT_FAILURE;
}
