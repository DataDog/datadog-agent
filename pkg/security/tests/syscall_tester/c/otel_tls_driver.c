// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Runs an otel-span-* command out of otel_tls_lib.c. Built three ways, so that
// otel_thread_ctx_v1 is reached through a different TLS access model each time:
// compiled together with the library (the variable lands in the executable's
// static TLS block), linked against it at startup, or, with USE_DLOPEN, loading
// the shared object named by the first argument after the process is running.

#define _GNU_SOURCE

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef USE_DLOPEN
#include <dlfcn.h>
#else
#include "otel_tls_lib.h"
#endif

typedef int (*otel_command_fn)(int argc, char **argv);

struct command_entry {
    const char *name;
    const char *symbol;
#ifndef USE_DLOPEN
    otel_command_fn fn;
#endif
};

#ifdef USE_DLOPEN
#define OTEL_COMMAND(name, symbol) {name, #symbol}
#else
#define OTEL_COMMAND(name, symbol) {name, #symbol, symbol}
#endif

static const struct command_entry commands[] = {
    OTEL_COMMAND("otel-span-open", otel_span_open),
    OTEL_COMMAND("otel-span-open-wait", otel_span_open_wait),
    OTEL_COMMAND("otel-span-exec", otel_span_exec),
    OTEL_COMMAND("otel-span-fork-exec", otel_span_fork_exec),
    OTEL_COMMAND("otel-span-fork-open", otel_span_fork_open),
    OTEL_COMMAND("otel-span-open-invalid", otel_span_open_invalid),
    OTEL_COMMAND("otel-span-open-null-ptr", otel_span_open_null_ptr),
    OTEL_COMMAND("otel-span-exec-invalid", otel_span_exec_invalid),
    OTEL_COMMAND("otel-span-exec-null-ptr", otel_span_exec_null_ptr),
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

#ifdef USE_DLOPEN

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

#else

int main(int argc, char **argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    // Reaching main is proof enough: a startup link against a fixture the
    // runtime cannot load never gets here.
    if (strcmp(argv[1], "check") == 0) {
        return EXIT_SUCCESS;
    }

    const struct command_entry *command = lookup_command(argv[1]);
    if (command == NULL) {
        return EXIT_FAILURE;
    }

    int exit_code = command->fn(argc - 1, argv + 1);
    if (exit_code != EXIT_SUCCESS) {
        fprintf(stderr, "Command `%s` failed: %d (errno: %s)\n", argv[1], exit_code, strerror(errno));
    }
    return exit_code;
}

#endif
