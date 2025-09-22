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
    bpf_printk("PID is %d", tgid);
    bpf_printk("Registering SSH session ID: %llu", request.key.id);

    pid_cache_entry->user_session_id = request.key.id;
    bpf_map_update_elem(&user_sessions, &request.key, &request.session, BPF_ANY);
    return 0;
};

int __attribute__((always_inline)) register_ssh_user_session(struct pam_event_t *event) {
    if (!event) {
        return 0;
    }
    
    u64 ssh_session_id = rand64();

    // Create the key using the correct structure
    struct user_session_key_t key = {
        .id = ssh_session_id,
        .cursor = 1,
    };
    
    // Create the session data using the correct structure (256 bytes total)
    struct user_session_t session = {
        .session_type = 2,
    };
    
    // Copy username to bytes 0-31 of data field
    bpf_probe_read(&session.data[0], 32, event->user);
    // Copy hostIP to bytes 32-47 of data field (16 bytes)
    bpf_probe_read(&session.data[32], 16, event->hostIP);
    bpf_printk("Data: %s", session.data[32]);   
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct pid_cache_t *pid_cache_entry = get_pid_cache(tgid);
    if (pid_cache_entry == NULL) {
        // exit early, this process isn't tracked by CWS, this shouldn't happen
        return 0;
    }
    bpf_printk("PID is %d", tgid);

    // check if a session already exists for the current pid
    if (pid_cache_entry->user_session_id != 0) {
        bpf_printk("Existing session ID: %llu", pid_cache_entry->user_session_id);
        // does the current session ID match the input one ?
        if (pid_cache_entry->user_session_id != key.id) {
            bpf_printk("Key ID is %llu", key.id);
            // ignore, is someone trying to compromise the user context ?
            return 0;
        }
    }

    // if we're here, either the existing session ID matches or there is no session ID yet. Either way, persist the
    // provided data.
    bpf_printk("Registering SSH session ID: %llu", ssh_session_id);
    pid_cache_entry->user_session_id = ssh_session_id;
    bpf_map_update_elem(&user_sessions, &key, &session, BPF_ANY);
    return 0;
}

#endif
