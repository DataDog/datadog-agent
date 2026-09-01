// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Runs an otel-node-span-* command out of otel_nodejs_lib.c, dlopen'd once the
// process is already running -- how a Node addon is loaded, and therefore the
// only access path worth driving here. Which underlying TLS access model that
// lands on is the native tester's business (see otel_tls_driver.c).

#define _GNU_SOURCE

#include <dlfcn.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct command_entry {
    const char *name;
    const char *symbol;
};

static const struct command_entry commands[] = {
    {"otel-node-span-open", "otel_node_span_open"},
    {"otel-node-span-open-chained", "otel_node_span_open_chained"},
    {"otel-node-span-open-deep", "otel_node_span_open_deep"},
    {"otel-node-span-exec", "otel_node_span_exec"},
    {"otel-node-span-fork-exec", "otel_node_span_fork_exec"},
    {"otel-node-span-open-invalid", "otel_node_span_open_invalid"},
    {"otel-node-span-open-no-context", "otel_node_span_open_no_context"},
    {"otel-node-span-open-als-absent", "otel_node_span_open_als_absent"},
    {"otel-node-span-open-no-writer", "otel_node_span_open_no_writer"},
};

static const struct command_entry *lookup_command(const char *name) {
    for (size_t i = 0; i < sizeof(commands) / sizeof(commands[0]); i++) {
        if (strcmp(name, commands[i].name) == 0) {
            return &commands[i];
        }
    }
    fprintf(stderr, "Unknown command: %s\n", name);
    return NULL;
}

typedef int (*otel_command_fn)(int argc, char **argv);

// The fixture is dlopen'd even for `check`: the driver itself references only
// the libc symbols it uses, so it can start where the fixture cannot load, and
// the test would then select a variant whose very first dlopen fails.
static void *load_fixture(const char *path) {
    void *handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (handle == NULL) {
        fprintf(stderr, "dlopen %s failed: %s\n", path, dlerror());
    }
    return handle;
}

int main(int argc, char **argv) {
    if (argc <= 2) {
        fprintf(stderr, "Usage: %s <fixture.so> <command> [args...]\n", argv[0]);
        return EXIT_FAILURE;
    }

    void *handle = load_fixture(argv[1]);
    if (handle == NULL) {
        return EXIT_FAILURE;
    }

    argc -= 1;
    argv += 1;

    if (strcmp(argv[1], "check") == 0) {
        dlclose(handle);
        return EXIT_SUCCESS;
    }

    const struct command_entry *command = lookup_command(argv[1]);
    if (command == NULL) {
        dlclose(handle);
        return EXIT_FAILURE;
    }

    dlerror();
    otel_command_fn fn = (otel_command_fn)dlsym(handle, command->symbol);
    const char *err = dlerror();
    if (err != NULL) {
        fprintf(stderr, "dlsym %s failed: %s\n", command->symbol, err);
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
