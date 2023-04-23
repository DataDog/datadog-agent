// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_TYPES_H
#define DATADOG_AGENT_RTLOADER_TYPES_H
#include <stdbool.h>
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
#    define _strdup(x) strdupe(x)
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

// From
//    datadog-agent\pkg\diagnose\diagnosis\loader.go and
//    datadog-agent\rtloader\include\rtloader_types.h
typedef enum {
    DATADOG_AGENT_RTLOADER_DIAGNOSIS_SUCCESS = 0,
    DATADOG_AGENT_RTLOADER_DIAGNOSIS_NOT_ENABLED = 1,
    DATADOG_AGENT_RTLOADER_DIAGNOSIS_FAIL = 2,
    DATADOG_AGENT_RTLOADER_DIAGNOSIS_WARNING = 3,
    DATADOG_AGENT_RTLOADER_DIAGNOSIS_UNEXPECTED_ERROR = 4,
} DiagnosisResult;

// All pointers point to portion of memory contained completely within diagnoses_t buffer
// size_t or pointers are needed to avoid Go vs. CGO misalignments
typedef struct diagnosis_s {
    // required fields
    size_t result;
    char *name;
    char *diagnosis;

    // optional fields
    char *category;
    char *description;
    char *remediation;
    char *raw_error;
} diagnosis_t;

// Self-contained array of diagnosis. All pointers point to portion of memory contained
// completely within the diagnoses_t buffer. size_t or pointers are needed to avoid Go vs. CGO misalignments
typedef struct diagnoses_s {
    size_t byteCout;
    size_t diangosesCount;
    diagnosis_t *diagnosesItems;
} diagnoses_t;

typedef struct py_info_s {
    const char *version; // returned by Py_GetInfo(); is static string owned by python
    char *path; // allocated within getPyInfo()
} py_info_t;

typedef enum {
    DATADOG_AGENT_TRACE = 7,
    DATADOG_AGENT_DEBUG = 10,
    DATADOG_AGENT_INFO = 20,
    DATADOG_AGENT_WARNING = 30,
    DATADOG_AGENT_ERROR = 40,
    DATADOG_AGENT_CRITICAL = 50
} log_level_t;

typedef struct pymem_stats_s {
    size_t inuse, alloc;
} pymem_stats_t;

/*
 * custom builtins
 */

// aggregator
//
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
// (tracemalloc_enabled)
typedef bool (*cb_tracemalloc_enabled_t)(void);
// (message, level)
typedef void (*cb_log_t)(char *, int);
// (check_id, name, value)
typedef void (*cb_set_check_metadata_t)(char *, char *, char *);
// (hostname, source_type_name, list of tags)
typedef void (*cb_set_external_tags_t)(char *, char *, char **);
// (key, value)
typedef void (*cb_write_persistent_cache_t)(char *, char *);
// (value)
typedef char *(*cb_read_persistent_cache_t)(char *);
// (sql_query, options, error_message)
typedef char *(*cb_obfuscate_sql_t)(char *, char *, char **);
// (exec_plan, normalize, error_message)
typedef char *(*cb_obfuscate_sql_exec_plan_t)(char *, bool, char **);
// ()
typedef double (*cb_get_process_start_time_t)(void);

// _util
// (argv, env, stdout, stderr, ret_code, exception)
typedef void (*cb_get_subprocess_output_t)(char **, char **, char **, char **, int *, char **);

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
// (container_name, image_name, namespace, bool_result)
typedef int (*cb_is_excluded_t)(char *, char *, char *);

#ifdef __cplusplus
}
#endif
#endif
