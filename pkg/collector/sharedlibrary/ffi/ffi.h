// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef FFI_H
#define FFI_H

#include <stdbool.h>

#include "rtloader_types.h"

// aggregator_t stores callbacks used by shared library checks
typedef struct aggregator_s {
    cb_submit_metric_t cb_submit_metric;
    cb_submit_service_check_t cb_submit_service_check;
    cb_submit_event_t cb_submit_event;
    cb_submit_histogram_bucket_t cb_submit_histogram_bucket;
    cb_submit_event_platform_event_t cb_submit_event_platform_event;
} aggregator_t;

// run function, entrypoint of checks
// (check_id, init_config, instance_config, callbacks, error)
typedef void (run_function_t)(char *, char *, char *, const aggregator_t *, const char **);

// shared library check version function
// (error)
typedef const char *(version_function_t)(const char **);

// library_t contains handle of the shared library and pointers to its symbols
typedef struct library_s {
    void *handle;                // handle of the shared library
    run_function_t *run;         // pointer to the `Run` symbol
    version_function_t *version; // pointer to the `Version` symbol
} library_t;

// shared library interface functions
library_t load_shared_library(const char *lib_path, const char **error);
void close_shared_library(void *lib_handle, const char **error);
void run_shared_library(run_function_t *run_ptr, char *check_id, char *init_config, char *instance_config, aggregator_t *aggregator, const char **error);
const char *get_version_shared_library(version_function_t *get_version_ptr, const char **error);

#endif
