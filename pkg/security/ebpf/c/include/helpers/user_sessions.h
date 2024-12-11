#ifndef _HELPERS_USER_SESSIONS_H_
#define _HELPERS_USER_SESSIONS_H_

#include "maps.h"

#include "process.h"

int __attribute__((always_inline)) handle_register_user_session(void *data) {
    struct user_session_request_t request = {};
    bpf_probe_read(&request, sizeof(request), data);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct pid_cache_t *pid_cache_entry = get_pid_cache(tgid);
    if (pid_cache_entry == NULL) {
        // exit early, this process isn't tracked by CWS, this shouldn't happen
        return 0;
    }

    // check if a session already exists for the current pid
    if (pid_cache_entry->user_session_id != 0) {
        // does the current session ID match the input one ?
        if (pid_cache_entry->user_session_id != request.key.id) {
            // ignore request, is someone trying to compromise the user context ?
            return 0;
        }
    }

    // if we're here, either the existing session ID matches or there is no session ID yet. Either way, persist the
    // provided data.
    pid_cache_entry->user_session_id = request.key.id;
    bpf_map_update_elem(&user_sessions, &request.key, &request.session, BPF_ANY);

    return 0;
}

#endif
