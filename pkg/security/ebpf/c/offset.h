#define MIN_PID_OFFSET 32
#define MAX_PID_OFFSET 256

struct bpf_map_def SEC("maps/pid_offset") pid_offset = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

SEC("kprobe/get_pid_task")
int kprobe_get_pid_task(struct pt_regs *ctx) {
    struct pid *pid = (struct pid *) PT_REGS_PARM1(ctx);
    if (!pid) {
        return 0;
    }

    u64 pid_expected;
    LOAD_CONSTANT("pid_expected", pid_expected);

    u32 offset = 0, success = 0;

#pragma unroll
    for (int i = MIN_PID_OFFSET; i != MAX_PID_OFFSET; i++) {
        u32 root_nr = 0;

        bpf_probe_read(&root_nr, sizeof(root_nr), (void *)pid + offset);
        if (root_nr == pid_expected) {
            // found it twice, thus error
            if (success) {
                return 0;
            }
            success = offset;
        }

        offset++;
    }

    if (success) {
        u32 key = 0;
        bpf_map_update_elem(&pid_offset, &key, &success, BPF_ANY);
    }

    return 0;
}