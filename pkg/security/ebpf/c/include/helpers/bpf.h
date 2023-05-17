#ifndef _BPF_H
#define _BPF_H

#include "constants/enums.h"
#include "events_definition.h"
#include "maps.h"

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

#endif
