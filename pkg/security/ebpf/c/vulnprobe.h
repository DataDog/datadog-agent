#ifndef _VULNPROBE_H_
#define _VULNPROBE_H_

#include "defs.h"
#include "process.h"

struct vulnprobe_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;

    u64 id;
};

__attribute__((always_inline)) static u64 load_vuln_id() {
    u64 vuln_id = 0;
    LOAD_CONSTANT("vuln_id", vuln_id);
    return vuln_id;
}

__attribute__((always_inline)) static u64 load_vuln_rule_id() {
    u64 vuln_id = 0;
    LOAD_CONSTANT("rule_vuln_id", vuln_id);
    return vuln_id;
}

SEC("uprobe/vuln_detector")
int uprobe_vuln_detector(void *ctx)
{
    u64 id = load_vuln_id();
    u64 rule_id = load_vuln_rule_id();
    bpf_printk("vulnprobe id %lu / rule_id %lu\n", id, rule_id);

    // TODO: probe args

    /* constuct and send the event */
    struct vulnprobe_event_t event = {
        .id = id,
    };
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_UPROBE, event);
    return 0;
}

#endif /* _VULNPROBE_H_ */
