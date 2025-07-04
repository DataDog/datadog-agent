#ifndef _MAPS_H_
#define _MAPS_H_

#include "map-defs.h"

#include "constants/custom.h"
#include "constants/enums.h"
#include "structs/all.h"

#define BPF_SK_MAP(_name, _value_type)         \
    struct {                                   \
        __uint(type, BPF_MAP_TYPE_SK_STORAGE); \
        __type(value, _value_type);            \
        __uint(map_flags, BPF_F_NO_PREALLOC);  \
        __type(key, u32);                      \
    } _name SEC(".maps");

BPF_ARRAY_MAP(path_id, u32, PATH_ID_MAP_SIZE)
BPF_ARRAY_MAP(enabled_events, u64, 1)
BPF_ARRAY_MAP(buffer_selector, u32, 5)
BPF_ARRAY_MAP(dr_erpc_buffer, char[DR_ERPC_BUFFER_LENGTH * 2], 1)
BPF_ARRAY_MAP(inode_disc_revisions, u32, REVISION_ARRAY_SIZE)
BPF_ARRAY_MAP(discarders_revision, u32, 1)
BPF_ARRAY_MAP(filter_policy, struct policy_t, EVENT_MAX)
BPF_ARRAY_MAP(mmap_flags_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(mmap_protection_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(mprotect_vm_protection_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(mprotect_req_protection_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(open_flags_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(selinux_enforce_status, u16, 2)
BPF_ARRAY_MAP(splice_entry_flags_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(splice_exit_flags_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(bpf_cmd_approvers, struct u64_flags_filter_t, 1)
BPF_ARRAY_MAP(sysctl_action_approvers, struct u32_flags_filter_t, 1)
BPF_ARRAY_MAP(connect_addr_family_approvers, struct u64_flags_filter_t, 1)
BPF_ARRAY_MAP(syscalls_stats_enabled, u32, 1)
BPF_ARRAY_MAP(syscall_ctx_gen_id, u32, 1)
BPF_ARRAY_MAP(syscall_ctx, char[MAX_SYSCALL_CTX_SIZE], MAX_SYSCALL_CTX_ENTRIES)
BPF_ARRAY_MAP(global_rate_limiters, struct rate_limiter_ctx, 1)
BPF_ARRAY_MAP(filtered_dns_rcodes, u16, 1)

BPF_HASH_MAP(activity_dumps_config, u64, struct activity_dump_config, 1) // max entries will be overridden at runtime
BPF_HASH_MAP(activity_dump_config_defaults, u32, struct activity_dump_config, 5)
BPF_HASH_MAP(traced_cgroups, struct path_key_t, u64, 1) // max entries will be overridden at runtime
BPF_HASH_MAP(cgroup_wait_list, struct path_key_t, u64, 1) // max entries will be overridden at runtime
BPF_HASH_MAP(traced_pids, u32, u64, 8192) // max entries will be overridden at runtime
BPF_HASH_MAP(basename_approvers, struct basename_t, struct event_mask_filter_t, 255)
BPF_HASH_MAP(register_netdevice_cache, u64, struct register_netdevice_cache_t, 1024)
BPF_HASH_MAP(netdevice_lookup_cache, u64, struct device_ifindex_t, 1024)
BPF_HASH_MAP(fd_link_pid, u8, u32, 1)
BPF_HASH_MAP(security_profiles, container_id_t, struct security_profile_t, 1) // max entries will be overriden at runtime
BPF_HASH_MAP(secprofs_syscalls, u64, struct security_profile_syscalls_t, 1) // max entries will be overriden at runtime
BPF_HASH_MAP(auid_approvers, u32, struct event_mask_filter_t, 128)
BPF_HASH_MAP(auid_range_approvers, u32, struct u32_range_filter_t, EVENT_MAX)
BPF_HASH_MAP(active_flows_spin_locks, u32, struct active_flows_spin_lock_t, 1) // max entry will be overridden at runtime
BPF_HASH_MAP(inode_file, u64, struct file_t, 32)

BPF_HASH_MAP_FLAGS(active_flows, u32, struct active_flows_t, 1, BPF_F_NO_PREALLOC) // max entry will be overridden at runtime
BPF_HASH_MAP_FLAGS(inet_bind_args, u64, struct inet_bind_args_t, 1, BPF_F_NO_PREALLOC) // max entries will be overridden at runtime

BPF_LRU_MAP(activity_dump_rate_limiters, u64, struct rate_limiter_ctx, 1) // max entries will be overridden at runtime
BPF_LRU_MAP(pid_rate_limiters, u32, struct rate_limiter_ctx, 1) // max entries will be overridden at runtime
BPF_LRU_MAP(mount_ref, u32, struct mount_ref_t, 64000)
BPF_LRU_MAP(bpf_maps, u32, struct bpf_map_t, 4096)
BPF_LRU_MAP(bpf_progs, u32, struct bpf_prog_t, 4096)
BPF_LRU_MAP(tgid_fd_map_id, struct bpf_tgid_fd_t, u32, 4096)
BPF_LRU_MAP(tgid_fd_prog_id, struct bpf_tgid_fd_t, u32, 4096)
BPF_LRU_MAP(proc_cache, u64, struct proc_cache_t, 1) // max entries will be overridden at runtime
BPF_LRU_MAP(pid_cache, u32, struct pid_cache_t, 1) // max entries will be overridden at runtime
BPF_LRU_MAP(pid_ignored, u32, u32, 16738)
BPF_LRU_MAP(exec_pid_transfer, u32, u64, 512)
BPF_LRU_MAP(netns_cache, u32, u32, 40960)
BPF_LRU_MAP(span_tls, u32, struct span_tls_t, 1) // max entries will be overridden at runtime
BPF_LRU_MAP(inode_discarders, struct inode_discarder_t, struct inode_discarder_params_t, 4096)
BPF_LRU_MAP(flow_pid, struct pid_route_t, struct pid_route_entry_t, 10240)
BPF_LRU_MAP(conntrack, struct namespaced_flow_t, struct namespaced_flow_t, 4096) // TODO: size should be updated dynamically with "nf_conntrack_max"
BPF_LRU_MAP(io_uring_ctx_pid, void *, u64, 2048)
BPF_LRU_MAP(veth_state_machine, u64, struct veth_state_t, 1024)
BPF_LRU_MAP(veth_devices, struct device_ifindex_t, struct device_t, 1024)
BPF_LRU_MAP(syscall_monitor, struct syscall_monitor_key_t, struct syscall_monitor_entry_t, 2048)
BPF_LRU_MAP(syscall_table, struct syscall_table_key_t, u8, 50)
BPF_LRU_MAP(kill_list, u32, u32, 32)
BPF_LRU_MAP(user_sessions, struct user_session_key_t, struct user_session_t, 1024)
BPF_LRU_MAP(dentry_resolver_inputs, u64, struct dentry_resolver_input_t, 256)
BPF_LRU_MAP(ns_flow_to_network_stats, struct namespaced_flow_t, struct network_stats_t, 4096) // TODO: size should be updated dynamically with "nf_conntrack_max"
BPF_LRU_MAP(sock_meta, void *, struct sock_meta_t, 4096);
BPF_LRU_MAP(dns_responses_sent_to_userspace, u16, struct dns_responses_sent_to_userspace_lru_entry_t, 1024)

BPF_LRU_MAP_FLAGS(tasks_in_coredump, u64, u8, 64, BPF_F_NO_COMMON_LRU)
BPF_LRU_MAP_FLAGS(syscalls, u64, struct syscall_cache_t, 1, BPF_F_NO_COMMON_LRU) // max entries will be overridden at runtime
BPF_LRU_MAP_FLAGS(pathnames, struct path_key_t, struct path_leaf_t, 1, BPF_F_NO_COMMON_LRU) // edited

BPF_SK_MAP(sk_storage_meta, struct sock_meta_t);

BPF_PERCPU_ARRAY_MAP(dr_erpc_state, struct dr_erpc_state_t, 1)
BPF_PERCPU_ARRAY_MAP(cgroup_tracing_event_gen, struct cgroup_tracing_event_t, EVENT_GEN_SIZE)
BPF_PERCPU_ARRAY_MAP(cgroup_prefix, cgroup_prefix_t, 1)
BPF_PERCPU_ARRAY_MAP(fb_discarder_stats, struct discarder_stats_t, EVENT_LAST_DISCARDER + 1)
BPF_PERCPU_ARRAY_MAP(bb_discarder_stats, struct discarder_stats_t, EVENT_LAST_DISCARDER + 1)
BPF_PERCPU_ARRAY_MAP(fb_approver_stats, struct approver_stats_t, EVENT_LAST_APPROVER + 1)
BPF_PERCPU_ARRAY_MAP(bb_approver_stats, struct approver_stats_t, EVENT_LAST_APPROVER + 1)
BPF_PERCPU_ARRAY_MAP(fb_dns_stats, struct dns_receiver_stats_t, 1)
BPF_PERCPU_ARRAY_MAP(bb_dns_stats, struct dns_receiver_stats_t, 1)
BPF_PERCPU_ARRAY_MAP(str_array_buffers, struct str_array_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(process_event_gen, struct process_event_t, EVENT_GEN_SIZE)
BPF_PERCPU_ARRAY_MAP(dr_erpc_stats_fb, struct dr_erpc_stats_t, 6)
BPF_PERCPU_ARRAY_MAP(dr_erpc_stats_bb, struct dr_erpc_stats_t, 6)
BPF_PERCPU_ARRAY_MAP(is_discarded_by_inode_gen, struct is_discarded_by_inode_t, 1)
BPF_PERCPU_ARRAY_MAP(dns_event, struct dns_event_t, 1)
BPF_PERCPU_ARRAY_MAP(dns_response_event, union dns_responses_t, 1)
BPF_PERCPU_ARRAY_MAP(imds_event, struct imds_event_t, 1)
BPF_PERCPU_ARRAY_MAP(packets, struct packet_t, 1)
BPF_PERCPU_ARRAY_MAP(selinux_write_buffer, struct selinux_write_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(is_new_kthread, u32, 1)
BPF_PERCPU_ARRAY_MAP(syscalls_stats, struct syscalls_stats_t, EVENT_MAX)
BPF_PERCPU_ARRAY_MAP(raw_packet_event, struct raw_packet_event_t, 1)
BPF_PERCPU_ARRAY_MAP(network_flow_monitor_event_gen, struct network_flow_monitor_event_t, 1)
BPF_PERCPU_ARRAY_MAP(active_flows_gen, struct active_flows_t, 1)
BPF_PERCPU_ARRAY_MAP(raw_packet_enabled, u32, 1)
BPF_PERCPU_ARRAY_MAP(sysctl_event_gen, struct sysctl_event_t, 1)
BPF_PERCPU_ARRAY_MAP(on_demand_event_gen, struct on_demand_event_t, 1)
BPF_PERCPU_ARRAY_MAP(setsockopt_event, struct setsockopt_event_t, 1)

BPF_PROG_ARRAY(args_envs_progs, 3)
BPF_PROG_ARRAY(dentry_resolver_kprobe_or_fentry_callbacks, EVENT_MAX)
BPF_PROG_ARRAY(dentry_resolver_tracepoint_callbacks, EVENT_MAX)
BPF_PROG_ARRAY(dentry_resolver_kprobe_or_fentry_progs, 6)
BPF_PROG_ARRAY(dentry_resolver_tracepoint_progs, 3)
BPF_PROG_ARRAY(classifier_router, 10)
BPF_PROG_ARRAY(sys_exit_progs, 64)
BPF_PROG_ARRAY(raw_packet_classifier_router, 32)
BPF_PROG_ARRAY(flush_network_stats_progs, 2)
BPF_PROG_ARRAY(open_ret_progs, 1)

BPF_PERF_EVENT_ARRAY_MAP(events, u32)
BPF_PERCPU_ARRAY_MAP(events_stats, struct perf_map_stats_t, EVENT_MAX)

#if USE_RING_BUFFER == 1
BPF_ARRAY_MAP(events_ringbuf_stats, u64, 1)
#endif

#endif
