#include <stdbool.h>

typedef enum {
    DATADOG_AGENT_RTLOADER_GAUGE = 0,
    DATADOG_AGENT_RTLOADER_RATE,
    DATADOG_AGENT_RTLOADER_COUNT,
    DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT,
    DATADOG_AGENT_RTLOADER_COUNTER,
    DATADOG_AGENT_RTLOADER_HISTOGRAM,
    DATADOG_AGENT_RTLOADER_HISTORATE
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

typedef struct aggregator_s {
    cb_submit_metric_t cb_submit_metric;
    cb_submit_service_check_t cb_submit_service_check;
    cb_submit_event_t cb_submit_event;
    cb_submit_histogram_bucket_t cb_submit_histogram_bucket;
    cb_submit_event_platform_event_t cb_submit_event_platform_event;
} aggregator_t;
