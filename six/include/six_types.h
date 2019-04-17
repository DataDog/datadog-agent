// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_TYPES_H
#define DATADOG_AGENT_SIX_TYPES_H
#include <string.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifndef DATADOG_AGENT_SIX_API
#    ifdef DATADOG_AGENT_SIX_TEST
#        define DATADOG_AGENT_SIX_API
#    elif _WIN32
#        define DATADOG_AGENT_SIX_API __declspec(dllexport)
#    else
#        if __GNUC__ >= 4
#            define DATADOG_AGENT_SIX_API __attribute__((visibility("default")))
#        else
#            define DATADOG_AGENT_SIX_API
#        endif
#    endif
#endif

#ifndef WIN32
#    define _strdup(x) strdup(x)
#endif

typedef enum six_gilstate_e { DATADOG_AGENT_SIX_GIL_LOCKED, DATADOG_AGENT_SIX_GIL_UNLOCKED } six_gilstate_t;

typedef enum {
    DATADOG_AGENT_SIX_GAUGE = 0,
    DATADOG_AGENT_SIX_RATE,
    DATADOG_AGENT_SIX_COUNT,
    DATADOG_AGENT_SIX_MONOTONIC_COUNT,
    DATADOG_AGENT_SIX_COUNTER,
    DATADOG_AGENT_SIX_HISTOGRAM,
    DATADOG_AGENT_SIX_HISTORATE
} metric_type_t;

typedef struct event_t {
    char *title;
    char *text;
    long ts;
    char *priority;
    char *host;
    char **tags;
    int tags_num;
    char *alert_type;
    char *aggregation_key;
    char *source_type_name;
    char *event_type;
} event_t;

/*
 * custom builtins
 */

// aggregator
//
// (id, metric_type, metric_name, value, tags, tags_len, hostname)
typedef void (*cb_submit_metric_t)(char *, metric_type_t, char *, float, char **, int, char *);
// (id, sc_name, status, tags, tags_len, hostname, message)
typedef void (*cb_submit_service_check_t)(char *, char *, int, char **, int, char *, char *);
// (id, event)
typedef void (*cb_submit_event_t)(char *, event_t *);

// datadog_agent
//
// (version)
typedef void (*cb_get_version_t)(char **);
// (key, json_result)
typedef void (*cb_get_config_t)(char *, char **);
// (json_result)
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
// (argv, argc, raise, output)
typedef void (*cb_get_subprocess_output_t)(char **, int, int, char **);

// CGO API
//
typedef void (*cb_cgo_free_t)(void *);

// tagger
//
// (id, highCard)
typedef void (*cb_get_tags_t)(char *, int, char **);

// kubeutil
//
// (json_result)
typedef void (*cb_get_connection_info_t)(char **);

// containers
//
// (container_name, image_name, bool_result)
typedef void (*cb_is_excluded_t)(char *, char *, int *);

#ifdef __cplusplus
}
#endif
#endif
