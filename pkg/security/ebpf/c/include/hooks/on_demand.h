#ifndef _HOOKS_ON_DEMAND_H_
#define _HOOKS_ON_DEMAND_H_

#include "helpers/span_fill.h"

enum param_kind_t {
	PARAM_NO_ACTION,
	PARAM_KIND_INTEGER,
	PARAM_KIND_NULL_STR,
};

#define param_parsing_regular(idx) \
	u64 param##idx##kind; \
    LOAD_CONSTANT("param" #idx "kind", param##idx##kind); \
                                             \
	switch (param##idx##kind) { \
	case PARAM_KIND_INTEGER: \
		value = CTX_PARM##idx(ctx); \
		bpf_probe_read(&event->data[(idx - 1) * ON_DEMAND_PER_ARG_SIZE], sizeof(value), &value); \
		break; \
	case PARAM_KIND_NULL_STR: \
		buf = &event->data[(idx - 1) * ON_DEMAND_PER_ARG_SIZE]; \
		path = (char *)CTX_PARM##idx(ctx); \
		bpf_probe_read_str(buf, ON_DEMAND_PER_ARG_SIZE, path); \
		break; \
	}

#define param_parsing_syscall(idx) \
	u64 param##idx##kind; \
    LOAD_CONSTANT("param" #idx "kind", param##idx##kind); \
           \
	u64 arg##idx; \
	bpf_probe_read(&arg##idx, sizeof(arg##idx), &SYSCALL64_PT_REGS_PARM##idx(ctx)); \
                                             \
	switch (param##idx##kind) { \
	case PARAM_KIND_INTEGER: \
		bpf_probe_read(&event->data[(idx - 1) * ON_DEMAND_PER_ARG_SIZE], sizeof(arg##idx), &arg##idx); \
		break; \
	case PARAM_KIND_NULL_STR: \
		buf = &event->data[(idx - 1) * ON_DEMAND_PER_ARG_SIZE]; \
		path = (char *)arg##idx; \
		bpf_probe_read_str(buf, ON_DEMAND_PER_ARG_SIZE, path); \
		break; \
	}

#define HOOK_ON_DEMAND HOOK_ENTRY("parse_args")

struct on_demand_event_t* __attribute__((always_inline)) get_on_demand_event() {
	// The event is staged in the shared span_fill slot (built and emitted within
	// this same program, so no cross-hook persistence is needed). SPAN_FILL_EVENT
	// zeroes it; the span context is attached later by the tail-called
	// fill_span_and_send program.
	struct on_demand_event_t* evt = SPAN_FILL_EVENT(struct on_demand_event_t, EVENT_ON_DEMAND);
	if (!evt) {
		return NULL;
	}

	u64 synth_id;
    LOAD_CONSTANT("synth_id", synth_id);
	evt->synth_id = synth_id;

	struct proc_cache_t *entry = fill_process_context(&evt->process);
    fill_cgroup_context(entry, &evt->cgroup);

	return evt;
}

HOOK_ON_DEMAND
int hook_on_demand(ctx_t *ctx) {
	struct on_demand_event_t *event = get_on_demand_event();
	if (!event) return 0;

	char *path;
	char *buf;
	u64 value;

	param_parsing_regular(1);
	param_parsing_regular(2);
	param_parsing_regular(3);
	param_parsing_regular(4);
	param_parsing_regular(5);
	param_parsing_regular(6);

	span_fill_tail_call(ctx, KPROBE_OR_FENTRY_TYPE);

    return 0;
}

HOOK_ON_DEMAND
int hook_on_demand_syscall(ctx_t *ptctx) {
	struct pt_regs *ctx = (struct pt_regs *) CTX_PARM1(ptctx);
    if (!ctx) return 0;

	struct on_demand_event_t *event = get_on_demand_event();
	if (!event) return 0;

	char *path;
	char *buf;

	param_parsing_syscall(1);
	param_parsing_syscall(2);
	param_parsing_syscall(3);
	param_parsing_syscall(4);
	param_parsing_syscall(5);
	param_parsing_syscall(6);

	span_fill_tail_call(ptctx, KPROBE_OR_FENTRY_TYPE);

    return 0;
}

#endif
