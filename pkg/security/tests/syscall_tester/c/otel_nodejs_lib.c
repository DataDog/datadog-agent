// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Stands in for a Node.js process publishing an OTel thread context: exports the
// discovery thread-local a Node addon does, points it at an object graph shaped
// like the V8 one a record hangs off, and runs the syscall the eBPF reader is
// expected to observe. Built into a shared object the driver dlopens, which is how
// the real writer is loaded; see tasks/security_agent.py.

#define _GNU_SOURCE

#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#include "otel_nodejs_common.h"
#include "otel_nodejs_lib.h"

// The discovery struct, exported so the agent's resolver finds it in the shared
// object's dynamic symbol table. The Node addon publishes the same symbol, and
// like it this is left zeroed until the writer runs on the thread.
__attribute__((visibility("default")))
__thread struct otel_thread_ctx_nodejs otel_thread_ctx_nodejs_v1;

// otel_nodejs_scenario is what a command leaves for the reader to walk.
enum otel_nodejs_scenario {
    otel_nodejs_valid,       // a record the reader must report
    otel_nodejs_chained,     // the same, behind another key in its bucket
    otel_nodejs_deep,        // the same, behind more keys than the reader follows
    otel_nodejs_invalid,     // a record whose valid byte is 0
    otel_nodejs_no_context,  // an asynchronous context holding no record
    otel_nodejs_als_absent,  // a frame our AsyncLocalStorage is not in
    otel_nodejs_no_writer,   // a thread the writer never ran on
};

struct otel_nodejs_opts {
    char **argv;
    int memfd;
    enum otel_nodejs_scenario scenario;
    // path_index is where the file to open, or the program to exec, sits in
    // argv: the commands publishing no ids take none.
    int path_index;
};

static void publish_discovery(struct otel_nodejs_graph *graph) {
    otel_thread_ctx_nodejs_v1 = graph->discovery;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}

static void clear_discovery(void) {
    memset(&otel_thread_ctx_nodejs_v1, 0, sizeof(otel_thread_ctx_nodejs_v1));
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}

// prepare_nodejs_context publishes what a reader needs, in the order libdatadog
// publishes it, then waits for the agent to resolve this process before the record
// becomes reachable.
static int prepare_nodejs_context(struct otel_nodejs_graph *graph, char **argv, int *memfd,
                                  enum otel_nodejs_scenario scenario) {
    if (otel_nodejs_publish_process_ctx() < 0) {
        return -1;
    }

    *memfd = otel_nodejs_create_tracer_memfd();
    if (*memfd < 0) {
        return -1;
    }

    usleep(500000);
    if (scenario == otel_nodejs_no_writer) {
        return 0;
    }

    enum otel_nodejs_entry_placement placement = otel_nodejs_entry_head;
    switch (scenario) {
    case otel_nodejs_chained:
        placement = otel_nodejs_entry_chained;
        break;
    case otel_nodejs_deep:
        placement = otel_nodejs_entry_deep;
        break;
    case otel_nodejs_als_absent:
        placement = otel_nodejs_entry_absent;
        break;
    default:
        break;
    }
    otel_nodejs_build(graph, placement);

    if (scenario != otel_nodejs_no_context && scenario != otel_nodejs_als_absent) {
        otel_fill_record(&graph->record, argv[1], argv[2]);
        if (scenario == otel_nodejs_invalid) {
            graph->record.header.valid = 0;
            __atomic_signal_fence(__ATOMIC_SEQ_CST);
        }
    }
    if (scenario == otel_nodejs_no_context) {
        otel_nodejs_clear_context(graph);
    }

    publish_discovery(graph);
    return 0;
}

static int open_test_path(const char *path) {
    int fd = open(path, O_CREAT, 0777);
    if (fd < 0) {
        perror("open");
        return -1;
    }
    close(fd);
    unlink(path);
    return 0;
}

static void *thread_nodejs_open(void *data) {
    struct otel_nodejs_opts *opts = (struct otel_nodejs_opts *)data;
    struct otel_nodejs_graph graph;

    if (prepare_nodejs_context(&graph, opts->argv, &opts->memfd, opts->scenario) < 0) {
        return NULL;
    }

    open_test_path(opts->argv[opts->path_index]);

    clear_discovery();
    return NULL;
}

static void *thread_nodejs_exec(void *data) {
    struct otel_nodejs_opts *opts = (struct otel_nodejs_opts *)data;
    struct otel_nodejs_graph graph;

    if (prepare_nodejs_context(&graph, opts->argv, &opts->memfd, opts->scenario) < 0) {
        return NULL;
    }

    execv(opts->argv[opts->path_index], opts->argv + opts->path_index);
    perror("execv");
    clear_discovery();
    return NULL;
}

// run_on_thread publishes from a thread of its own, the way a Node worker thread
// would: the discovery struct is per-thread, and dies with it.
static int run_on_thread(void *(*fn)(void *), char **argv, enum otel_nodejs_scenario scenario, int path_index) {
    struct otel_nodejs_opts opts = {.argv = argv, .memfd = -1, .scenario = scenario, .path_index = path_index};
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

static int span_open(int argc, char **argv, const char *usage, enum otel_nodejs_scenario scenario) {
    if (argc < 4) {
        fprintf(stderr, "Usage: %s <trace_id> <span_id> <file_path>\n", usage);
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_nodejs_open, argv, scenario, 3);
}

int otel_node_span_open(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open", otel_nodejs_valid);
}

int otel_node_span_open_chained(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open-chained", otel_nodejs_chained);
}

int otel_node_span_open_deep(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open-deep", otel_nodejs_deep);
}

int otel_node_span_open_invalid(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open-invalid", otel_nodejs_invalid);
}

int otel_node_span_open_no_context(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open-no-context", otel_nodejs_no_context);
}

int otel_node_span_open_als_absent(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open-als-absent", otel_nodejs_als_absent);
}

int otel_node_span_open_no_writer(int argc, char **argv) {
    return span_open(argc, argv, "otel-node-span-open-no-writer", otel_nodejs_no_writer);
}

int otel_node_span_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-node-span-exec <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }
    return run_on_thread(thread_nodejs_exec, argv, otel_nodejs_valid, 3);
}

int otel_node_span_fork_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Usage: otel-node-span-fork-exec <trace_id> <span_id> <exec_path> [args...]\n");
        return EXIT_FAILURE;
    }

    int memfd = -1;
    struct otel_nodejs_graph graph;
    if (prepare_nodejs_context(&graph, argv, &memfd, otel_nodejs_valid) < 0) {
        return EXIT_FAILURE;
    }

    pid_t child = fork();
    if (child < 0) {
        perror("fork");
        clear_discovery();
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

    clear_discovery();
    close(memfd);
    return WIFEXITED(status) ? WEXITSTATUS(status) : EXIT_FAILURE;
}
