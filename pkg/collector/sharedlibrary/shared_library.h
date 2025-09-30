// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef SHARED_LIBRARY_H
#define SHARED_LIBRARY_H

/*! \file shared_library.h
    \brief Definition of types and declaration of functions used by
    the shared library loader.

    These definitions are kept in a separated file because they need
    to be included in multiple files.
*/

#include <stdbool.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

// shared libraries handler
#ifdef _WIN32
#    include <Windows.h>
#else
#    include <dlfcn.h>
#endif

// shared libraries extension
#if __linux__
#    define LIB_EXTENSION "so"
#elif __APPLE__
#    define LIB_EXTENSION "dylib"
#elif __FreeBSD__
#    define LIB_EXTENSION "so"
#elif _WIN32
#    define LIB_EXTENSION "dll"
#else
#    error Platform not supported
#endif

// metric types
typedef enum {
    GAUGE = 0,
    RATE,
    COUNT,
    MONOTONIC_COUNT,
    COUNTER,
    HISTOGRAM,
    HISTORATE
} metric_type_t;

typedef struct event_s {
    char *title;
    char *text;
    long ts;
    char *priority;
    char *host;
    char **tags;
    char *alert_type;
    char *aggregation_key;
    char *source_type_name;
    char *event_type;
} event_t;

// (id, metric_type, metric_name, value, tags, hostname, flush_first_value)
typedef void (*cb_submit_metric_t)(char *, metric_type_t, char *, double, char **, char *, bool);
// (id, sc_name, status, tags, hostname, message)
typedef void (*cb_submit_service_check_t)(char *, char *, int, char **, char *, char *);
// (id, event)
typedef void (*cb_submit_event_t)(char *, event_t *);
// (id, metric_name, value, lower_bound, upper_bound, monotonic, hostname, tags, flush_first_value)
typedef void (*cb_submit_histogram_bucket_t)(char *, char *, long long, float, float, int, char *, char **, bool);
// (id, event, event_type)
typedef void (*cb_submit_event_platform_event_t)(char *, char *, int, char *);

// aggregator_t stores every callback used by shared libraries checks
typedef struct aggregator_s {
    cb_submit_metric_t cb_submit_metric;
    cb_submit_service_check_t cb_submit_service_check;
    cb_submit_event_t cb_submit_event;
    cb_submit_histogram_bucket_t cb_submit_histogram_bucket;
    cb_submit_event_platform_event_t cb_submit_event_platform_event;
} aggregator_t;

// run function callback, entrypoint of checks
// (check_id, init_config, instance_config, callbacks)
typedef char *(run_function_t)(char *, char *, char *, const aggregator_t *);

// handles_t contains pointers to shared library and its Run symbol
typedef struct handles_s {
    void *lib;              // handle to the shared library
    run_function_t *run;    // handle to the run function symbol
} handles_t;

#ifdef _WIN32
static handles_t load_shared_library(const char *lib_name, const char **error) {
	handles_t lib_handles;

    // resolve the library full name
    size_t lib_full_name_length = strlen(lib_name) + strlen(LIB_EXTENSION) + 2;
    char *lib_full_name = (char *)malloc(lib_full_name_length);
	if (!lib_full_name) {
		*error = strdup("memory allocation for library name failed");
		goto done;
	}
	snprintf(lib_full_name, lib_full_name_length, "%s.%s", lib_name, LIB_EXTENSION);

    // load the library
    lib_handles.lib = LoadLibraryA(lib_full_name);
    if (!lib_handles.lib) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to open shared library, error code: %d", error_code);
		*error = strdup(error_msg);
		goto done;
    }

    // get symbol pointers of 'Run' and 'Free' functions
    lib_handles.run = (run_function_t *)GetProcAddress(lib_handles.lib, "Run");
    if (!lib_handles.run) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to get shared library 'Run' symbol, error code: %d", error_code);
		*error = strdup(error_msg);
		goto done;
    }

done:
	free(lib_full_name);
	return lib_handles;
}

static void close_shared_library(void *lib_handle) {
	// verify pointer
	if (!lib_handle) {
		return;
	}
    
	FreeLibrary(lib_handle);
}
#else
static handles_t load_shared_library(const char *lib_name, const char **error) {
	handles_t lib_handles;
    const char *dlsym_error = NULL;

    // resolve the library full name
    size_t lib_full_name_length = strlen(lib_name) + strlen(LIB_EXTENSION) + 2;
    char *lib_full_name = (char *)malloc(lib_full_name_length);
	if (!lib_full_name) {
		*error = strdup("memory allocation for library name failed");
		goto done;
	}
	snprintf(lib_full_name, lib_full_name_length, "%s.%s", lib_name, LIB_EXTENSION);

    // load the library
    lib_handles.lib = dlopen(lib_full_name, RTLD_LAZY | RTLD_GLOBAL);
    dlsym_error = dlerror();
    if (dlsym_error) {
		*error = strdup(dlsym_error);
		goto done;
    }

    // get symbol pointer of 'Run' function
    lib_handles.run = (run_function_t *)dlsym(lib_handles.lib, "Run");
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handles.lib);
		*error = strdup(dlsym_error);
		goto done;
    }

done:
	free(lib_full_name);
	return lib_handles;
}

static void close_shared_library(void *lib_handle) {
	// verify pointer
	if (!lib_handle) {
		return;
	}

	dlclose(lib_handle);
}
#endif

static char *run_shared_library(run_function_t *run_handle, char *check_id, char *init_config, char *instance_config, aggregator_t *aggregator) {
	// verify pointers
    if (!run_handle) {
        return strdup("pointer to shared library 'Run' symbol is NULL");
    }

    // run the shared library check and return any error has occurred
    char *error = (run_handle)(check_id, init_config, instance_config, aggregator);
    if (error) {
        return strdup(error);
    }

    return NULL;
}
#endif
