#ifndef _HOOKS_BPF_H_
#define _HOOKS_BPF_H_

#include "constants/offsets/bpf.h"
#include "constants/syscall_macro.h"
#include "helpers/bpf.h"
#include "helpers/discarders.h"
#include "helpers/process.h"
#include "helpers/syscalls.h"

__attribute__((always_inline)) void send_bpf_event(void *ctx, struct syscall_cache_t *syscall) {
    struct bpf_event_t event = {
        .syscall.retval = syscall->bpf.retval,
        .cmd = syscall->bpf.cmd,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    u32 id = 0;

    // select map if applicable
    if (syscall->bpf.map_id != 0) {
        id = syscall->bpf.map_id;
        struct bpf_map_t *map = bpf_map_lookup_elem(&bpf_maps, &id);
        if (map != NULL) {
            event.map = *map;
        }
    }

    // select prog if applicable
    if (syscall->bpf.prog_id != 0) {
        id = syscall->bpf.prog_id;
        struct bpf_prog_t *prog = bpf_map_lookup_elem(&bpf_progs, &id);
        if (prog != NULL) {
            event.prog = *prog;
        }
    }

    if (event.cmd == BPF_PROG_LOAD || event.cmd == BPF_MAP_CREATE) {
        // fill metadata from syscall arguments
        fill_from_syscall_args(syscall, &event);
    }

    // send event
    send_event(ctx, EVENT_BPF, event);
}

SYSCALL_KPROBE3(bpf, int, cmd, union bpf_attr __user *, uattr, unsigned int, size) {
    struct policy_t policy = fetch_policy(EVENT_BPF);
    if (is_discarded_by_process(policy.mode, EVENT_BPF)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_BPF,
        .bpf = {
            .cmd = cmd,
        }
    };
    bpf_probe_read(&syscall.bpf.attr, sizeof(syscall.bpf.attr), &uattr);

    cache_syscall(&syscall);

    return 0;
}

__attribute__((always_inline)) int sys_bpf_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_BPF);
    if (!syscall) {
        return 0;
    }

    syscall->bpf.retval = retval;

    // save file descriptor <-> map_id mapping if applicable
    if (syscall->bpf.map_id != 0 || syscall->bpf.prog_id != 0) {
        save_obj_fd(syscall);
    }

    // populate map_id or prog_id if applicable
    populate_map_id_and_prog_id(syscall);

    // send monitoring event
    send_bpf_event(ctx, syscall);
    return 0;
}

SYSCALL_KRETPROBE(bpf) {
    return sys_bpf_ret(ctx, (int)PT_REGS_RC(ctx));
}

SEC("kprobe/security_bpf_map")
int kprobe_security_bpf_map(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_BPF);
    if (!syscall) {
        return 0;
    }

    struct bpf_map *map = (struct bpf_map *)PT_REGS_PARM1(ctx);

    // collect relevant map metadata
    struct bpf_map_t m = {};
    bpf_probe_read(&m.id, sizeof(m.id), (void *)map + get_bpf_map_id_offset());
    bpf_probe_read(&m.name, sizeof(m.name), (void *)map + get_bpf_map_name_offset());
    bpf_probe_read(&m.map_type, sizeof(m.map_type), (void *)map + get_bpf_map_type_offset());

    // save map metadata
    bpf_map_update_elem(&bpf_maps, &m.id, &m, BPF_ANY);

    // update context
    syscall->bpf.map_id = m.id;
    return 0;
}

SEC("kprobe/security_bpf_prog")
int kprobe_security_bpf_prog(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_BPF);
    if (!syscall) {
        return 0;
    }

    struct bpf_prog *prog = (struct bpf_prog *)PT_REGS_PARM1(ctx);
    struct bpf_prog_aux *prog_aux = 0;
    bpf_probe_read(&prog_aux, sizeof(prog_aux), (void *)prog + get_bpf_prog_aux_offset());

    // collect relevant prog metadata
    struct bpf_prog_t p = {};
    bpf_probe_read(&p.id, sizeof(p.id), (void *)prog_aux + get_bpf_prog_aux_id_offset());
    bpf_probe_read(&p.prog_type, sizeof(p.prog_type), (void *)prog + get_bpf_prog_type_offset());
    if (get_bpf_prog_attach_type_offset() > 0) {
        bpf_probe_read(&p.attach_type, sizeof(p.attach_type), (void *)prog + get_bpf_prog_attach_type_offset());
    }
    bpf_probe_read(&p.name, sizeof(p.name), (void *)prog_aux + get_bpf_prog_aux_name_offset());
    bpf_probe_read(&p.tag, sizeof(p.tag), (void *)prog + get_bpf_prog_tag_offset());

    // update context
    syscall->bpf.prog_id = p.id;

    // add prog helpers
    p.helpers[0] = syscall->bpf.helpers[0];
    p.helpers[1] = syscall->bpf.helpers[1];
    p.helpers[2] = syscall->bpf.helpers[2];

    // save prog metadata
    bpf_map_update_elem(&bpf_progs, &p.id, &p, BPF_ANY);
    return 0;
}

SEC("kprobe/check_helper_call")
int kprobe_check_helper_call(struct pt_regs *ctx) {
    int func_id = 0;
    struct syscall_cache_t *syscall = peek_syscall(EVENT_BPF);
    if (!syscall) {
        return 0;
    }

    u64 input = get_check_helper_call_input();
    if (input == CHECK_HELPER_CALL_FUNC_ID) {
        func_id = (int)PT_REGS_PARM2(ctx);
    } else if (input == CHECK_HELPER_CALL_INSN) {
        struct bpf_insn *insn = (struct bpf_insn *)PT_REGS_PARM2(ctx);
        bpf_probe_read(&func_id, sizeof(func_id), &insn->imm);
    }

    if (func_id >= 128) {
        syscall->bpf.helpers[2] |= (u64) 1 << (func_id - 128);
    } else if (func_id >= 64) {
        syscall->bpf.helpers[1] |= (u64) 1 << (func_id - 64);
    } else if (func_id >= 0) {
        syscall->bpf.helpers[0] |= (u64) 1 << (func_id);
    }
    return 0;
}

SEC("tracepoint/handle_sys_bpf_exit")
int tracepoint_handle_sys_bpf_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_bpf_ret(args, args->ret);
}

#endif
