#ifndef SHARED_LIBRARY_TYPES_H
#define SHARED_LIBRARY_TYPES_H

#include <stdbool.h>

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

// config passed to the check
typedef struct submit_callbacks_s {
    cb_submit_metric_t cb_submit_metric;
    cb_submit_service_check_t cb_submit_service_check;
    cb_submit_event_t cb_submit_event;
    cb_submit_histogram_bucket_t cb_submit_histogram_bucket;
    cb_submit_event_platform_event_t cb_submit_event_platform_event;
} submit_callbacks_t;

// run function callback
// (instance string, callbacks)
typedef char *(run_function_t)(char *, const submit_callbacks_t *);

// free function callback
// (string to free)
typedef void(free_function_t)(char *);

// library file and symbols pointers
typedef struct shared_library_handles_s {
    void *lib; // handle to the shared library
    run_function_t *run; // handle to the run function symbol
    free_function_t *free; // handle to the free function symbol
} shared_library_handles_t;

#endif