#ifndef _BPF_MONITORING_H_
#define _BPF_MONITORING_H_

#include "syscalls.h"

#define CHECK_HELPER_CALL_FUNC_ID 1
#define CHECK_HELPER_CALL_INSN 2

u64 __attribute__((always_inline)) get_check_helper_call_input(void) {
    u64 input;
    LOAD_CONSTANT("check_helper_call_input", input);
    return input;
}

u64 __attribute__((always_inline)) get_bpf_map_id_offset(void) {
    u64 bpf_map_id_offset;
    LOAD_CONSTANT("bpf_map_id_offset", bpf_map_id_offset);
    return bpf_map_id_offset;
}

u64 __attribute__((always_inline)) get_bpf_map_name_offset(void) {
    u64 bpf_map_name_offset;
    LOAD_CONSTANT("bpf_map_name_offset", bpf_map_name_offset);
    return bpf_map_name_offset;
}

u64 __attribute__((always_inline)) get_bpf_map_type_offset(void) {
    u64 bpf_map_type_offset;
    LOAD_CONSTANT("bpf_map_type_offset", bpf_map_type_offset);
    return bpf_map_type_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_aux_offset(void) {
    u64 bpf_prog_aux_offset;
    LOAD_CONSTANT("bpf_prog_aux_offset", bpf_prog_aux_offset);
    return bpf_prog_aux_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_aux_id_offset(void) {
    u64 bpf_prog_aux_id_offset;
    LOAD_CONSTANT("bpf_prog_aux_id_offset", bpf_prog_aux_id_offset);
    return bpf_prog_aux_id_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_type_offset(void) {
    u64 bpf_prog_type_offset;
    LOAD_CONSTANT("bpf_prog_type_offset", bpf_prog_type_offset);
    return bpf_prog_type_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_attach_type_offset(void) {
    u64 bpf_prog_attach_type_offset;
    LOAD_CONSTANT("bpf_prog_attach_type_offset", bpf_prog_attach_type_offset);
    return bpf_prog_attach_type_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_aux_name_offset(void) {
    u64 bpf_prog_aux_name_offset;
    LOAD_CONSTANT("bpf_prog_aux_name_offset", bpf_prog_aux_name_offset);
    return bpf_prog_aux_name_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_tag_offset(void) {
    u64 bpf_prog_tag_offset;
    LOAD_CONSTANT("bpf_prog_tag_offset", bpf_prog_tag_offset);
    return bpf_prog_tag_offset;
}

struct bpf_map_t {
    u32 id;
    enum bpf_map_type map_type;
    char name[BPF_OBJ_NAME_LEN];
};

struct bpf_prog_t {
    u32 id;
    enum bpf_prog_type prog_type;
    enum bpf_attach_type attach_type;
    u32 padding;
    u64 helpers[3];
    char name[BPF_OBJ_NAME_LEN];
    char tag[BPF_TAG_SIZE];
};

struct bpf_tgid_fd_t {
    u32 tgid;
    u32 fd;
};

struct bpf_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct bpf_map_t map;
    struct bpf_prog_t prog;
    int cmd;
    u32 padding;
};

struct bpf_map_def SEC("maps/bpf_maps") bpf_maps = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct bpf_map_t),
    .max_entries = 4096,
};

struct bpf_map_def SEC("maps/bpf_progs") bpf_progs = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct bpf_prog_t),
    .max_entries = 4096,
};

struct bpf_map_def SEC("maps/tgid_fd_map_id") tgid_fd_map_id = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct bpf_tgid_fd_t),
    .value_size = sizeof(u32),
    .max_entries = 4096,
};

struct bpf_map_def SEC("maps/tgid_fd_prog_id") tgid_fd_prog_id = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct bpf_tgid_fd_t),
    .value_size = sizeof(u32),
    .max_entries = 4096,
};

__attribute__((always_inline)) void save_obj_fd(struct syscall_cache_t *syscall) {
    struct bpf_tgid_fd_t key = {
        .tgid = bpf_get_current_pid_tgid() >> 32,
        .fd = syscall->bpf.retval,
    };

    u32 id = 0;

    switch (syscall->bpf.cmd) {
    case BPF_MAP_CREATE:
    case BPF_MAP_GET_FD_BY_ID:
        id = syscall->bpf.map_id;
        bpf_map_update_elem(&tgid_fd_map_id, &key, &id, BPF_ANY);
        break;
    case BPF_PROG_LOAD:
    case BPF_PROG_GET_FD_BY_ID:
        id = syscall->bpf.prog_id;
        bpf_map_update_elem(&tgid_fd_prog_id, &key, &id, BPF_ANY);
        break;
    }
}

__attribute__((always_inline)) u32 fetch_map_id(int fd) {
    struct bpf_tgid_fd_t key = {
        .tgid = bpf_get_current_pid_tgid() >> 32,
        .fd = fd,
    };

    u32 *map_id = bpf_map_lookup_elem(&tgid_fd_map_id, &key);
    if (map_id == NULL) {
        return 0;
    }
    return *map_id;
}

__attribute__((always_inline)) u32 fetch_prog_id(int fd) {
    struct bpf_tgid_fd_t key = {
        .tgid = bpf_get_current_pid_tgid() >> 32,
        .fd = fd,
    };

    u32 *map_id = bpf_map_lookup_elem(&tgid_fd_prog_id, &key);
    if (map_id == NULL) {
        return 0;
    }
    return *map_id;
}

__attribute__((always_inline)) void populate_map_id_and_prog_id(struct syscall_cache_t *syscall) {
    int fd = 0;

    switch (syscall->bpf.cmd) {
    case BPF_MAP_LOOKUP_ELEM_CMD:
    case BPF_MAP_UPDATE_ELEM_CMD:
    case BPF_MAP_DELETE_ELEM_CMD:
    case BPF_MAP_LOOKUP_AND_DELETE_ELEM_CMD:
    case BPF_MAP_GET_NEXT_KEY_CMD:
    case BPF_MAP_FREEZE_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->map_fd);
        syscall->bpf.map_id = fetch_map_id(fd);
        break;
    case BPF_PROG_ATTACH_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->attach_bpf_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_PROG_DETACH_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->target_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_PROG_QUERY_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->query.target_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_PROG_TEST_RUN_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->test.prog_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_PROG_GET_NEXT_ID_CMD:
        bpf_probe_read(&syscall->bpf.prog_id, sizeof(syscall->bpf.prog_id), &syscall->bpf.attr->start_id);
        break;
    case BPF_MAP_GET_NEXT_ID_CMD:
        bpf_probe_read(&syscall->bpf.map_id, sizeof(syscall->bpf.prog_id), &syscall->bpf.attr->start_id);
        break;
    case BPF_OBJ_GET_INFO_BY_FD_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->info.bpf_fd);
        syscall->bpf.map_id = fetch_map_id(fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_OBJ_PIN_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->bpf_fd);
        syscall->bpf.map_id = fetch_map_id(fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_RAW_TRACEPOINT_OPEN_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->raw_tracepoint.prog_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_TASK_FD_QUERY_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->task_fd_query.fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_MAP_LOOKUP_BATCH_CMD:
    case BPF_MAP_LOOKUP_AND_DELETE_BATCH_CMD:
    case BPF_MAP_UPDATE_BATCH_CMD:
    case BPF_MAP_DELETE_BATCH_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->batch.map_fd);
        syscall->bpf.map_id = fetch_map_id(fd);
        break;
    case BPF_LINK_CREATE_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->link_create.prog_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_LINK_UPDATE_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->link_update.old_prog_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    case BPF_PROG_BIND_MAP_CMD:
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->prog_bind_map.map_fd);
        syscall->bpf.map_id = fetch_map_id(fd);
        bpf_probe_read(&fd, sizeof(fd), &syscall->bpf.attr->prog_bind_map.prog_fd);
        syscall->bpf.prog_id = fetch_prog_id(fd);
        break;
    }
}

__attribute__((always_inline)) void fill_from_syscall_args(struct syscall_cache_t *syscall, struct bpf_event_t *event) {
    switch (event->cmd) {
    case BPF_MAP_CREATE:
        bpf_probe_read(&event->map.map_type, sizeof(event->map.map_type), &syscall->bpf.attr->map_type);
        bpf_probe_read(&event->map.name, sizeof(event->map.name), &syscall->bpf.attr->map_name);
        break;
    case BPF_PROG_LOAD:
        bpf_probe_read(&event->prog.prog_type, sizeof(event->prog.prog_type), &syscall->bpf.attr->prog_type);
        bpf_probe_read(&event->prog.name, sizeof(event->prog.name), &syscall->bpf.attr->prog_name);
        bpf_probe_read(&event->prog.attach_type, sizeof(event->prog.attach_type), &syscall->bpf.attr->expected_attach_type);
        break;
    }
}

__attribute__((always_inline)) void send_bpf_event(void *ctx, struct syscall_cache_t *syscall) {
    struct bpf_event_t event = {
        .syscall.retval = syscall->bpf.retval,
        .event.async = 0,
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
