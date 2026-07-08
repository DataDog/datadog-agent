// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#define _GNU_SOURCE

#include <dlfcn.h>
#include <errno.h>
#include <limits.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "otel_tls_common.h"

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

    int memfd = otel_create_tracer_memfd();
    if (memfd < 0) {
        unload_initial_exec_fixture(&fixture);
        return EXIT_FAILURE;
    }

    usleep(500000);

    struct otel_record_with_attrs record;
    otel_fill_record(&record, argv[1], argv[2]);
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
