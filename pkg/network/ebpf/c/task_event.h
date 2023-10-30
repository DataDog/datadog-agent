#ifndef __TASK_EVENT_H
#define __TASK_EVENT_H

#define RETURN_IF_NOT_IN_SYSPROBE_TASK(prog_name)           \
    if (!event_in_task(prog_name)) {                        \
        return 0;                                           \
    }

static __always_inline __u32 systemprobe_dev() {
    __u64 val = 0;
    LOAD_CONSTANT("systemprobe_device", val);
    return (__u32)val;
}

static __always_inline __u32 systemprobe_ino() {
    __u64 val = 0;
    LOAD_CONSTANT("systemprobe_ino", val);
    return (__u32)val;
}

static __always_inline bool event_in_task(char *prog_name) {
    __u32 dev = systemprobe_dev();
    __u32 ino = systemprobe_ino();
    struct bpf_pidns_info ns = {};

    u64 error = bpf_get_ns_current_pid_tgid(dev, ino, &ns, sizeof(struct bpf_pidns_info));

    if (error) {
        log_debug("%s: err=event originates from outside current fargate task\n", prog_name);
    }

    return !error;
}

#endif