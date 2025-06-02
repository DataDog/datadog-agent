#ifndef _HOOKS_ON_DEMAND_H_
#define _HOOKS_ON_DEMAND_H_

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
	u32 key = 0;
	struct on_demand_event_t* evt = bpf_map_lookup_elem(&on_demand_event_gen, &key);
	if (!evt) {
		return NULL;
	}

	u64 synth_id;
    LOAD_CONSTANT("synth_id", synth_id);

	// make sure the event is clean
	evt->synth_id = synth_id;
	for (int i = 0; i < 6; i++) {
		u64 *ptr = (u64 *)(&evt->data[i * ON_DEMAND_PER_ARG_SIZE]);
		*ptr = 0;
	}

	struct proc_cache_t *entry = fill_process_context(&evt->process);
    fill_container_context(entry, &evt->container);
    fill_span_context(&evt->span);

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

	send_event_ptr(ctx, EVENT_ON_DEMAND, event);

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

	send_event_ptr(ptctx, EVENT_ON_DEMAND, event);

    return 0;
}

#endif
