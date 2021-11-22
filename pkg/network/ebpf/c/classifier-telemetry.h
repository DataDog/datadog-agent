#ifndef __CLASSIFIER_TELEMETRY_H
#define __CLASSIFIER_TELEMETRY_H

#include "classifier-maps.h"

#include "bpf_endian.h"

#include <linux/kconfig.h>
#include <net/sock.h>

enum classifier_telemetry_counter
{
    tail_call_failed,
    tls_flow_classified,
};

static __always_inline void increment_classifier_telemetry_count(enum classifier_telemetry_counter counter_name) {
    __u64 key = 0;
    classifier_telemetry_t *val = NULL;
    val = bpf_map_lookup_elem(&classifier_telemetry, &key);
    if (val == NULL) {
        return;
    }

    switch (counter_name) {
    case tail_call_failed:
        __sync_fetch_and_add(&val->tail_call_failed, 1);
        break;
    case tls_flow_classified:
        __sync_fetch_and_add(&val->tls_flow_classified, 1);
        break;
    }
}

#endif // __CLASSIFIER_TELEMETRY_H
