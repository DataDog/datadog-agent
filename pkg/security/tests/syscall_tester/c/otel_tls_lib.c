// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Publishes an OTel thread local context record from a TLS variable, then runs
// the syscall the eBPF reader is expected to observe -- or publishes a record
// the reader must reject, for the negative cases. Built both into the main
// executable and into a shared object, once per TLS access model the resolver
// has to handle; see tasks/security_agent.py.

#define _GNU_SOURCE

#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#include "otel_tls_common.h"
#include "otel_tls_lib.h"

// The local-dynamic access model only appears when the TLS variable is not
// preemptible, which needs it hidden; the resolver then has to find it in
// .symtab, the dynamic symbol table no longer carrying it.
#ifdef OTEL_TLS_HIDDEN
__attribute__((visibility("hidden")))
#else
__attribute__((visibility("default")))
#endif
__thread struct otel_thread_ctx_record *otel_thread_ctx_v1 = NULL;

// otel_record_mode selects what a command leaves in otel_thread_ctx_v1 before
// making its syscall.
enum otel_record_mode {
    otel_record_valid,   // a filled record, which the reader must report
    otel_record_invalid, // a record whose valid byte is 0, which it must reject
    otel_record_absent,  // nothing at all: the pointer stays NULL
};

struct otel_thread_opts {
    char **argv;
    int memfd;
    enum otel_record_mode mode;
    // path_index is where the file to open, or the program to exec, sits in
    // argv: the commands publishing no record take no ids either.
    int path_index;
};

static void publish_otel_record(struct otel_record_with_attrs *record) {
    otel_thread_ctx_v1 = NULL;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    otel_thread_ctx_v1 = &record->header;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}

static int prepare_otel_context(struct otel_record_with_attrs *record, char **argv, int *memfd,
                                enum otel_record_mode mode) {
    *memfd = otel_create_tracer_memfd();
    if (*memfd < 0) {
        return -1;
    }

    usleep(500000);
    if (mode == otel_record_absent) {
        return 0;
    }

    otel_fill_record(record, argv[1], argv[2]);
    if (mode == otel_record_invalid) {
        record->header.valid = 0;
        __atomic_signal_fence(__ATOMIC_SEQ_CST);
    }
    publish_otel_record(record);
    return 0;
}

static int open_test_path(const char *path, int unlink_after) {
    int fd = open(path, O_CREAT, 0777);
    if (fd < 0) {
        perror("open");
        return -1;
    }
    close(fd);
    if (unlink_after) {
        unlink(path);
    }
    return 0;
}

static void *thread_otel_open(void *data) {
    struct otel_thread_opts *opts = (struct otel_thread_opts *)data;
    struct otel_record_with_attrs record;

    if (prepare_otel_context(&record, opts->argv, &opts->memfd, opts->mode) < 0) {
        return NULL;
    }

    open_test_path(opts->argv[opts->path_index], 1);

    otel_thread_ctx_v1 = NULL;
    return NULL;
}

// run_on_thread publishes the record from a thread of its own, the way a traced
// application would, and the record dies with it.
static int run_on_thread(void *(*fn)(void *), char **argv, enum otel_record_mode mode, int path_index) {
    struct otel_thread_opts opts = {.argv = argv, .memfd = -1, .mode = mode, .path_index = path_index};
    pthread_t thread;

    if (pthread_create(&thread, NULL, fn, &opts) != 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    if (opts.memfd >= 0) {
        close(opts.memfd);
    }
    return EXIT_SUCCESS;
}

int otel_span_open(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-open <trace_id> <span_id> <file_path>\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_otel_open, argv, otel_record_valid, 3);
}

int otel_span_open_invalid(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-open-invalid <trace_id> <span_id> <file_path>\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_otel_open, argv, otel_record_invalid, 3);
}

int otel_span_open_null_ptr(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: otel-span-open-null-ptr <file_path>\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_otel_open, argv, otel_record_absent, 1);
}

static int wait_for_file(const char *path) {
    for (int i = 0; i < 1000; i++) {
        if (access(path, F_OK) == 0) {
            return 0;
        }
        usleep(10000);
    }
    fprintf(stderr, "timed out waiting for %s\n", path);
    return -1;
}

int otel_span_open_wait(int argc, char **argv) {
    if (argc < 6) {
        fprintf(stderr, "Usage: otel-span-open-wait <trace_id> <span_id> <ready_path> <continue_path> <file_path>\n");
        return EXIT_FAILURE;
    }

    int memfd = -1;
    struct otel_record_with_attrs record;
    if (prepare_otel_context(&record, argv, &memfd, otel_record_valid) < 0) {
        return EXIT_FAILURE;
    }

    if (open_test_path(argv[3], 0) < 0 || wait_for_file(argv[4]) < 0 || open_test_path(argv[5], 1) < 0) {
        otel_thread_ctx_v1 = NULL;
        close(memfd);
        return EXIT_FAILURE;
    }

    otel_thread_ctx_v1 = NULL;
    close(memfd);
    return EXIT_SUCCESS;
}

static void *thread_otel_exec(void *data) {
    struct otel_thread_opts *opts = (struct otel_thread_opts *)data;
    struct otel_record_with_attrs record;

    if (prepare_otel_context(&record, opts->argv, &opts->memfd, opts->mode) < 0) {
        return NULL;
    }

    execv(opts->argv[opts->path_index], opts->argv + opts->path_index);
    perror("execv");
    otel_thread_ctx_v1 = NULL;
    return NULL;
}

int otel_span_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-exec <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_otel_exec, argv, otel_record_valid, 3);
}

int otel_span_exec_invalid(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-exec-invalid <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_otel_exec, argv, otel_record_invalid, 3);
}

int otel_span_exec_null_ptr(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: otel-span-exec-null-ptr <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_otel_exec, argv, otel_record_absent, 1);
}

// otel_span_fork_open has a forked child which never execs, so it keeps the
// The fork publishes a record of its own, with a different span
// id, before making the observed syscall.
int otel_span_fork_open(int argc, char **argv) {
    if (argc < 5) {
        fprintf(stderr, "Usage: otel-span-fork-open <trace_id> <parent_span_id> <child_span_id> <file_path>\n");
        return EXIT_FAILURE;
    }

    int memfd = -1;
    struct otel_record_with_attrs record;
    if (prepare_otel_context(&record, argv, &memfd, otel_record_valid) < 0) {
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
        struct otel_record_with_attrs own;
        otel_fill_record(&own, argv[1], argv[3]);
        publish_otel_record(&own);

        _exit(open_test_path(argv[4], 1) < 0 ? EXIT_FAILURE : EXIT_SUCCESS);
    }

    int status;
    waitpid(child, &status, 0);

    otel_thread_ctx_v1 = NULL;
    close(memfd);
    return WIFEXITED(status) ? WEXITSTATUS(status) : EXIT_FAILURE;
}

int otel_span_fork_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-span-fork-exec <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }

    int memfd = -1;
    struct otel_record_with_attrs record;
    if (prepare_otel_context(&record, argv, &memfd, otel_record_valid) < 0) {
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
