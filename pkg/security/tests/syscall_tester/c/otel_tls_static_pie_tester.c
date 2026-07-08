// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#define _GNU_SOURCE

#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#include "otel_tls_common.h"

__attribute__((visibility("default"))) __thread struct otel_thread_ctx_record *otel_thread_ctx_v1 = NULL;

struct otel_thread_opts {
    char **argv;
    int memfd;
};

static void publish_otel_record(struct otel_record_with_attrs *record) {
    otel_thread_ctx_v1 = NULL;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    otel_thread_ctx_v1 = &record->header;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}

static int prepare_otel_context(struct otel_record_with_attrs *record, char **argv, int *memfd) {
    *memfd = otel_create_tracer_memfd();
    if (*memfd < 0) {
        return -1;
    }

    usleep(500000);
    otel_fill_record(record, argv[1], argv[2]);
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
