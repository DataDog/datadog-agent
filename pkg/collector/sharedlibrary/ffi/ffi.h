// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef FFI_H
#define FFI_H

#include <stdbool.h>

// ---- ACR-compatible types ----

// Event struct for slim checks. All string fields are const pointers and the
// timestamp is a signed long, matching the ACR check_run ABI.
typedef struct slim_event_s {
    const char *title;
    const char *text;
    long ts;
    const char *priority;
    const char *host;
    const char **tags;
    const char *alert_type;
    const char *aggregation_key;
    const char *source_type_name;
    const char *event_type;
} slim_event_t;

// Callback struct matching the ACR's callback::Callback layout.
// Each function pointer takes void *ctx as its first argument.
typedef struct callback_s {
    void (*submit_metric)(void *ctx, int metric_type, const char *name, double value, const char **tags, const char *hostname, int flush_first);
    void (*submit_service_check)(void *ctx, const char *name, int status, const char **tags, const char *hostname, const char *message);
    void (*submit_event)(void *ctx, const slim_event_t *event);
    void (*submit_histogram)(void *ctx, const char *name, long long value, float lower, float upper, int monotonic, const char *hostname, const char **tags, int flush_first);
    void (*submit_event_platform_event)(void *ctx, const char *event, int event_len, const char *event_type);
    void (*submit_log)(void *ctx, int level, const char *message);
} callback_t;

// ACR-compatible check_run function signature
// (init_config, instance_config, enrichment, callback, ctx, error)
typedef void (check_run_function_t)(const char *, const char *, const char *, const callback_t *, void *, const char **);

// shared library check version function
// (error)
typedef const char *(version_function_t)();

// library_t contains handle of the shared library and pointers to its symbols
typedef struct library_s {
    void *handle;                        // handle of the shared library
    check_run_function_t *check_run;     // pointer to the `check_run` symbol
    version_function_t *version;         // pointer to the `Version` symbol
} library_t;

// shared library interface functions
library_t load_shared_library(const char *lib_path, const char **error);
void close_shared_library(void *lib_handle, const char **error);
void run_check_shared_library(check_run_function_t *check_run_ptr, const char *init_config, const char *instance_config, const char *enrichment, const callback_t *callback, void *ctx, const char **error);
const char *get_version_shared_library(version_function_t *get_version_ptr, const char **error);

#endif
