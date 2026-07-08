// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#define _GNU_SOURCE

#include <dlfcn.h>
#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

typedef int (*otel_command_fn)(int argc, char **argv);

#ifndef OTEL_TLS_FIXTURE_SO
#define OTEL_TLS_FIXTURE_SO "libotel_tls_fixture.so"
#endif

struct command_entry {
    const char *name;
    const char *symbol;
};

static const struct command_entry commands[] = {
    {"otel-span-open", "otel_span_open"},
    {"otel-span-open-wait", "otel_span_open_wait"},
    {"otel-span-exec", "otel_span_exec"},
    {"otel-span-fork-exec", "otel_span_fork_exec"},
};

static const char *symbol_for_command(const char *name) {
    for (size_t i = 0; i < sizeof(commands) / sizeof(commands[0]); i++) {
        if (strcmp(name, commands[i].name) == 0) {
            return commands[i].symbol;
        }
    }
    return NULL;
}

static int fixture_path(char *path, size_t path_len) {
    char exe_path[PATH_MAX];
    ssize_t len = readlink("/proc/self/exe", exe_path, sizeof(exe_path) - 1);
    if (len < 0) {
        perror("readlink /proc/self/exe");
        return -1;
    }
    exe_path[len] = '\0';

    char *slash = strrchr(exe_path, '/');
    if (slash == NULL) {
        fprintf(stderr, "unable to find executable directory\n");
        return -1;
    }
    *slash = '\0';

    int written = snprintf(path, path_len, "%s/%s", exe_path, OTEL_TLS_FIXTURE_SO);
    if (written < 0 || (size_t)written >= path_len) {
        fprintf(stderr, "fixture path is too long\n");
        return -1;
    }
    return 0;
}

int main(int argc, char **argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    if (strcmp(argv[1], "check") == 0) {
        return EXIT_SUCCESS;
    }

    const char *symbol = symbol_for_command(argv[1]);
    if (symbol == NULL) {
        fprintf(stderr, "Unknown command: %s\n", argv[1]);
        return EXIT_FAILURE;
    }

    char path[PATH_MAX];
    if (fixture_path(path, sizeof(path)) < 0) {
        return EXIT_FAILURE;
    }

    void *handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (handle == NULL) {
        fprintf(stderr, "dlopen %s failed: %s\n", path, dlerror());
        return EXIT_FAILURE;
    }

    dlerror();
    otel_command_fn fn = (otel_command_fn)dlsym(handle, symbol);
    const char *err = dlerror();
    if (err != NULL) {
        fprintf(stderr, "dlsym %s failed: %s\n", symbol, err);
        dlclose(handle);
        return EXIT_FAILURE;
    }

    int exit_code = fn(argc - 1, argv + 1);
    dlclose(handle);
    if (exit_code != EXIT_SUCCESS) {
        fprintf(stderr, "Command `%s` failed: %d (errno: %s)\n", argv[1], exit_code, strerror(errno));
    }
    return exit_code;
}
