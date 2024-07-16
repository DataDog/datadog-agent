#ifndef _HOOKS_ON_DEMAND_H_
#define _HOOKS_ON_DEMAND_H_

#define PER_ARG_SIZE 64

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
		bpf_probe_read(&event.data[(idx - 1) * PER_ARG_SIZE], sizeof(value), &value); \
		break; \
	case PARAM_KIND_NULL_STR: \
		buf = &event.data[(idx - 1) * PER_ARG_SIZE]; \
		path = (char *)CTX_PARM##idx(ctx); \
		bpf_probe_read_str(buf, PER_ARG_SIZE, path); \
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
		bpf_probe_read(&event.data[(idx - 1) * PER_ARG_SIZE], sizeof(arg##idx), &arg##idx); \
		break; \
	case PARAM_KIND_NULL_STR: \
		buf = &event.data[(idx - 1) * PER_ARG_SIZE]; \
		path = (char *)arg##idx; \
		bpf_probe_read_str(buf, PER_ARG_SIZE, path); \
		break; \
	}

#define HOOK_ON_DEMAND HOOK_ENTRY("parse_args")

HOOK_ON_DEMAND
int hook_on_demand(ctx_t *ctx) {
	u64 synth_id;
    LOAD_CONSTANT("synth_id", synth_id);

	struct on_demand_event_t event = {
		.synth_id = synth_id,
	};

	struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

	char *path;
	char *buf;
	u64 value;

	param_parsing_regular(1);
	param_parsing_regular(2);
	param_parsing_regular(3);
	param_parsing_regular(4);

	send_event(ctx, EVENT_ON_DEMAND, event);

    return 0;
}

HOOK_ON_DEMAND
int hook_on_demand_syscall(ctx_t *ptctx) {
	struct pt_regs *ctx = (struct pt_regs *) CTX_PARM1(ptctx);
    if (!ctx) return 0;

	u64 synth_id;
    LOAD_CONSTANT("synth_id", synth_id);

	struct on_demand_event_t event = {
		.synth_id = synth_id,
	};

	struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

	char *path;
	char *buf;

	param_parsing_syscall(1);
	param_parsing_syscall(2);
	param_parsing_syscall(3);
	param_parsing_syscall(4);

	send_event(ptctx, EVENT_ON_DEMAND, event);

    return 0;
}

#endif
