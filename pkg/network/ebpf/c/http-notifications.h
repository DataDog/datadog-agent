#ifndef __HTTP_NOTIFICATIONS_H
#define __HTTP_NOTIFICATIONS_H

#include "bpf_helpers.h"
#include "http-types.h"
#include "http-maps.h"

static __always_inline void http_notify_batch(struct pt_regs *ctx) {
    u32 cpu = bpf_get_smp_processor_id();

    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &cpu);
    if (batch_state == NULL || batch_state->idx_to_notify == batch_state->idx) {
        // batch is not ready to be flushed
        return;
    }

    // It's important to zero the struct so we account for the padding
    // introduced by the compilation, otherwise you get a `invalid indirect read
    // from stack off`. Alternatively we can either use a #pragma pack directive
    // or try to manually add the padding to the struct definition. More
    // information in https://docs.cilium.io/en/v1.8/bpf/ under the
    // alignment/padding section
    http_batch_notification_t notification = { 0 };
    notification.cpu = cpu;
    notification.batch_idx = batch_state->idx_to_notify;

    bpf_perf_event_output(ctx, &http_notifications, cpu, &notification, sizeof(http_batch_notification_t));
    log_debug("http batch notification flushed: cpu: %d idx: %d\n", notification.cpu, notification.batch_idx);
    batch_state->idx_to_notify++;
}

#endif //__HTTP_NOTIFICATIONS_H
