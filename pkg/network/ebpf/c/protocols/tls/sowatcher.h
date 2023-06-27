#ifndef __SOWATCHER_H
#define __SOWATCHER_H

#include "protocols/tls/sowatcher-types.h"

static __always_inline void fill_path_safe(lib_path_t *path, const char *path_argument) {
#pragma unroll
    for (int i = 0; i < LIB_PATH_MAX_SIZE; i++) {
        bpf_probe_read_user(&path->buf[i], 1, &path_argument[i]);
        if (path->buf[i] == 0) {
            path->len = i;
            break;
        }
    }
}

static __always_inline void do_sys_open_helper_enter(const char *filename) {
    lib_path_t path = {0};
    if (bpf_probe_read_user_with_telemetry(path.buf, sizeof(path.buf), filename) >= 0) {
// Find the null character and clean up the garbage following it
#pragma unroll
        for (int i = 0; i < LIB_PATH_MAX_SIZE; i++) {
            if (path.len) {
                path.buf[i] = 0;
            } else if (path.buf[i] == 0) {
                path.len = i;
            }
        }
    } else {
        fill_path_safe(&path, filename);
    }

    // Bail out if the path size is larger than our buffer
    if (!path.len) {
        return;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    path.pid = pid_tgid >> 32;
    bpf_map_update_with_telemetry(open_at_args, &pid_tgid, &path, BPF_ANY);
    return;
}

static __always_inline void do_sys_open_helper_exit(exit_sys_openat_ctx *args) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // If file couldn't be opened, bail out
    if (args->ret < 0) {
        goto cleanup;
    }

    lib_path_t *path = bpf_map_lookup_elem(&open_at_args, &pid_tgid);
    if (path == NULL) {
        return;
    }

// Check the last 6 characters of the following libraries to ensure the file is a relevant `.so`.
// Libraries:
//    libssl.so -> ssl.so
// libcrypto.so -> pto.so
// libgnutls.so -> tls.so


    // TESTING
    bool is_shared_library = false;
#define match2chars(_base, _a,_b) (path->buf[_base+i] == _a && path->buf[_base+i+1] == _b)
#define match3chars(_base, _a,_b,_c) (path->buf[_base+i] == _a && path->buf[_base+i+1] == _b && path->buf[_base+i+2] == _c)
#define match4chars(_base, _a,_b,_c,_d) (path->buf[_base+i] == _a && path->buf[_base+i+1] == _b && path->buf[_base+i+2] == _c && path->buf[_base+i+3] == _d)
    // this would match regex [spt][stl][los]\.so
    int i = 0;
#pragma unroll
    for (i = 0; i < LIB_PATH_MAX_SIZE - (LIB_SO_SUFFIX_SIZE); i++) {
        if(
            /*
            ((path->buf[i] == 's') || (path->buf[i] == 'p') || (path->buf[i] == 't')) &&
            ((path->buf[i+1] == 's') || (path->buf[i+1] == 't') || (path->buf[i+1] == 'l')) &&
            ((path->buf[i+2] == 'l') || (path->buf[i+2] == 'o') || (path->buf[i+2] == 's')) &&
            */
            match3chars(3, '.','s','o')) {
            is_shared_library = true;
            break;
        }
    }
    if (!is_shared_library) {
        goto cleanup;
    }

    if (!match3chars(0, 's','s','l') && !match3chars(0, 'p','t','o') && !match3chars(0, 't','l','s')) {
        goto cleanup;
    }

    u32 cpu = bpf_get_smp_processor_id();
    bpf_perf_event_output_with_telemetry((void*)args, &shared_libraries, cpu, path, sizeof(lib_path_t));
cleanup:
    bpf_map_delete_elem(&open_at_args, &pid_tgid);
    return;
}

SEC("tracepoint/syscalls/sys_enter_openat")
int tracepoint__syscalls__sys_enter_openat(enter_sys_openat_ctx* args) {
    do_sys_open_helper_enter(args->filename);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_openat")
int tracepoint__syscalls__sys_exit_openat(exit_sys_openat_ctx *args) {
    do_sys_open_helper_exit(args);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_openat2")
int tracepoint__syscalls__sys_enter_openat2(enter_sys_openat2_ctx* args) {
    do_sys_open_helper_enter(args->filename);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_openat2")
int tracepoint__syscalls__sys_exit_openat2(exit_sys_openat_ctx *args) {
    do_sys_open_helper_exit(args);
    return 0;
}

#endif
