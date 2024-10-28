#ifndef _MAPS_H_
#define _MAPS_H_

#include "map-defs.h"

#include "constants/custom.h"
#include "constants/enums.h"
#include "structs/all.h"

BPF_ARRAY_MAP(path_id, u32, PATH_ID_MAP_SIZE)
BPF_ARRAY_MAP(enabled_events, u64, 1)
BPF_ARRAY_MAP(buffer_selector, u32, 4)
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
BPF_ARRAY_MAP(syscalls_stats_enabled, u32, 1)
BPF_ARRAY_MAP(syscall_ctx_gen_id, u32, 1)
BPF_ARRAY_MAP(syscall_ctx, char[MAX_SYSCALL_CTX_SIZE], MAX_SYSCALL_CTX_ENTRIES)

BPF_HASH_MAP(activity_dumps_config, u64, struct activity_dump_config, 1) // max entries will be overridden at runtime
BPF_HASH_MAP(activity_dump_config_defaults, u32, struct activity_dump_config, 1)
BPF_HASH_MAP(traced_cgroups, container_id_t, u64, 1) // max entries will be overridden at runtime
BPF_HASH_MAP(cgroup_wait_list, container_id_t, u64, 1) // max entries will be overridden at runtime
BPF_HASH_MAP(traced_pids, u32, u64, 8192) // max entries will be overridden at runtime
BPF_HASH_MAP(basename_approvers, struct basename_t, struct event_mask_filter_t, 255)
BPF_HASH_MAP(register_netdevice_cache, u64, struct register_netdevice_cache_t, 1024)
BPF_HASH_MAP(netdevice_lookup_cache, u64, struct device_ifindex_t, 1024)
BPF_HASH_MAP(fd_link_pid, u8, u32, 1)
BPF_HASH_MAP(security_profiles, container_id_t, struct security_profile_t, 1) // max entries will be overriden at runtime
BPF_HASH_MAP(secprofs_syscalls, u64, struct security_profile_syscalls_t, 1) // max entries will be overriden at runtime
BPF_HASH_MAP(auid_approvers, u32, struct event_mask_filter_t, 128)
BPF_HASH_MAP(auid_range_approvers, u32, struct u32_range_filter_t, EVENT_MAX)

BPF_LRU_MAP(activity_dump_rate_limiters, u64, struct activity_dump_rate_limiter_ctx, 1) // max entries will be overridden at runtime
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
BPF_LRU_MAP(span_tls, u32, struct span_tls_t, 4096)
BPF_LRU_MAP(inode_discarders, struct inode_discarder_t, struct inode_discarder_params_t, 4096)
BPF_LRU_MAP(flow_pid, struct pid_route_t, u32, 10240)
BPF_LRU_MAP(conntrack, struct namespaced_flow_t, struct namespaced_flow_t, 4096)
BPF_LRU_MAP(io_uring_ctx_pid, void *, u64, 2048)
BPF_LRU_MAP(veth_state_machine, u64, struct veth_state_t, 1024)
BPF_LRU_MAP(veth_devices, struct device_ifindex_t, struct device_t, 1024)
BPF_LRU_MAP(exec_file_cache, u64, struct file_t, 4096)
BPF_LRU_MAP(syscall_monitor, struct syscall_monitor_key_t, struct syscall_monitor_entry_t, 2048)
BPF_LRU_MAP(syscall_table, struct syscall_table_key_t, u8, 50)
BPF_LRU_MAP(kill_list, u32, u32, 32)
BPF_LRU_MAP(user_sessions, struct user_session_key_t, struct user_session_t, 1024)
BPF_LRU_MAP(dentry_resolver_inputs, u64, struct dentry_resolver_input_t, 256)

BPF_LRU_MAP_FLAGS(tasks_in_coredump, u64, u8, 64, BPF_F_NO_COMMON_LRU)
BPF_LRU_MAP_FLAGS(syscalls, u64, struct syscall_cache_t, 1, BPF_F_NO_COMMON_LRU) // max entries will be overridden at runtime
BPF_LRU_MAP_FLAGS(pathnames, struct path_key_t, struct path_leaf_t, 1, BPF_F_NO_COMMON_LRU) // edited

BPF_PERCPU_ARRAY_MAP(dr_erpc_state, struct dr_erpc_state_t, 1)
BPF_PERCPU_ARRAY_MAP(cgroup_tracing_event_gen, struct cgroup_tracing_event_t, EVENT_GEN_SIZE)
BPF_PERCPU_ARRAY_MAP(cgroup_prefix, cgroup_prefix_t, 1)
BPF_PERCPU_ARRAY_MAP(fb_discarder_stats, struct discarder_stats_t, EVENT_LAST_DISCARDER + 1)
BPF_PERCPU_ARRAY_MAP(bb_discarder_stats, struct discarder_stats_t, EVENT_LAST_DISCARDER + 1)
BPF_PERCPU_ARRAY_MAP(fb_approver_stats, struct approver_stats_t, EVENT_LAST_APPROVER + 1)
BPF_PERCPU_ARRAY_MAP(bb_approver_stats, struct approver_stats_t, EVENT_LAST_APPROVER + 1)
BPF_PERCPU_ARRAY_MAP(str_array_buffers, struct str_array_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(process_event_gen, struct process_event_t, EVENT_GEN_SIZE)
BPF_PERCPU_ARRAY_MAP(dr_erpc_stats_fb, struct dr_erpc_stats_t, 6)
BPF_PERCPU_ARRAY_MAP(dr_erpc_stats_bb, struct dr_erpc_stats_t, 6)
BPF_PERCPU_ARRAY_MAP(is_discarded_by_inode_gen, struct is_discarded_by_inode_t, 1)
BPF_PERCPU_ARRAY_MAP(dns_event, struct dns_event_t, 1)
BPF_PERCPU_ARRAY_MAP(imds_event, struct imds_event_t, 1)
BPF_PERCPU_ARRAY_MAP(packets, struct packet_t, 1)
BPF_PERCPU_ARRAY_MAP(selinux_write_buffer, struct selinux_write_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(is_new_kthread, u32, 1)
BPF_PERCPU_ARRAY_MAP(syscalls_stats, struct syscalls_stats_t, EVENT_MAX)
BPF_PERCPU_ARRAY_MAP(raw_packets, struct raw_packet_t, 1)

BPF_PROG_ARRAY(args_envs_progs, 3)
BPF_PROG_ARRAY(dentry_resolver_kprobe_or_fentry_callbacks, EVENT_MAX)
BPF_PROG_ARRAY(dentry_resolver_tracepoint_callbacks, EVENT_MAX)
BPF_PROG_ARRAY(dentry_resolver_kprobe_or_fentry_progs, 6)
BPF_PROG_ARRAY(dentry_resolver_tracepoint_progs, 3)
BPF_PROG_ARRAY(classifier_router, 100)
BPF_PROG_ARRAY(sys_exit_progs, 64)

#endif
