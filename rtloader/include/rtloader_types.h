// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_TYPES_H
#define DATADOG_AGENT_RTLOADER_TYPES_H
#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifndef DATADOG_AGENT_RTLOADER_API
#    ifdef DATADOG_AGENT_RTLOADER_TEST
#        define DATADOG_AGENT_RTLOADER_API
#    elif _WIN32
#        define DATADOG_AGENT_RTLOADER_API __declspec(dllexport)
#    else
#        if __GNUC__ >= 4
#            define DATADOG_AGENT_RTLOADER_API __attribute__((visibility("default")))
#        else
#            define DATADOG_AGENT_RTLOADER_API
#        endif
#    endif
#endif

#ifndef WIN32
#    define _strdup(x) strdup(x)
#endif

typedef enum rtloader_gilstate_e {
    DATADOG_AGENT_RTLOADER_GIL_LOCKED = 0,
    DATADOG_AGENT_RTLOADER_GIL_UNLOCKED
} rtloader_gilstate_t;

typedef enum {
    DATADOG_AGENT_RTLOADER_ALLOCATION = 0,
    DATADOG_AGENT_RTLOADER_FREE,
} rtloader_mem_ops_t;

typedef void *(*rtloader_malloc_t)(size_t);
typedef void (*rtloader_free_t)(void *);

typedef enum {
    DATADOG_AGENT_RTLOADER_GAUGE = 0,
    DATADOG_AGENT_RTLOADER_RATE,
    DATADOG_AGENT_RTLOADER_COUNT,
    DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT,
    DATADOG_AGENT_RTLOADER_COUNTER,
    DATADOG_AGENT_RTLOADER_HISTOGRAM,
    DATADOG_AGENT_RTLOADER_HISTORATE
} metric_type_t;

typedef enum {
    DATADOG_AGENT_RTLOADER_TAGGER_LOW = 0,
    DATADOG_AGENT_RTLOADER_TAGGER_ORCHESTRATOR,
    DATADOG_AGENT_RTLOADER_TAGGER_HIGH,
} TaggerCardinality;

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

typedef struct py_info_s {
    const char *version;
    char *path;
} py_info_t;

/*
 * custom builtins
 */

// aggregator
//
// (id, metric_type, metric_name, value, tags, hostname)
typedef void (*cb_submit_metric_t)(char *, metric_type_t, char *, float, char **, char *);
// (id, sc_name, status, tags, hostname, message)
typedef void (*cb_submit_service_check_t)(char *, char *, int, char **, char *, char *);
// (id, event)
typedef void (*cb_submit_event_t)(char *, event_t *);

// datadog_agent
//
// (version)
typedef void (*cb_get_version_t)(char **);
// (key, yaml_result)
typedef void (*cb_get_config_t)(char *, char **);
// (yaml_result)
typedef void (*cb_headers_t)(char **);
// (hostname)
typedef void (*cb_get_hostname_t)(char **);
// (clustername)
typedef void (*cb_get_clustername_t)(char **);
// (message, level)
typedef void (*cb_log_t)(char *, int);
// (hostname, source_type_name, list of tags)
typedef void (*cb_set_external_tags_t)(char *, char *, char **);

// _util
// (argv, argc, raise, stdout, stderr, ret_code, exception)
typedef void (*cb_get_subprocess_output_t)(char **, char **, char **, int *, char **);

// CGO API
//
// memory
//
typedef void (*cb_cgo_free_t)(void *);
typedef void (*cb_memory_tracker_t)(void *, size_t sz, rtloader_mem_ops_t op);

// tagger
//
// (id, highCard)
typedef char **(*cb_tags_t)(char *, int);

// kubeutil
//
// (yaml_result)
typedef void (*cb_get_connection_info_t)(char **);

// containers
//
// (container_name, image_name, bool_result)
typedef int (*cb_is_excluded_t)(char *, char *);

#ifdef __cplusplus
}
#endif
#endif
