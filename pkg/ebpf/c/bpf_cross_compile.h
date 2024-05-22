#ifndef __BPF_CROSS_COMPILE__
#define __BPF_CROSS_COMPILE__

#ifdef COMPILE_CORE
#define bpf_helper_exists(fn) bpf_core_enum_value_exists(enum bpf_func_id, fn)
#endif

#ifdef COMPILE_RUNTIME
#include <uapi/linux/bpf.h>
#include <linux/version.h>

#if !defined(__BPF_FUNC_MAPPER)
#define __E_BPF_FUNC_map_lookup_elem false
#define __E_BPF_FUNC_map_update_elem false
#define __E_BPF_FUNC_map_delete_elem false
#define __E_BPF_FUNC_probe_read false
#define __E_BPF_FUNC_ktime_get_ns false
#define __E_BPF_FUNC_trace_printk false
#define __E_BPF_FUNC_get_prandom_u32 false
#define __E_BPF_FUNC_get_smp_processor_id false
#define __E_BPF_FUNC_skb_store_bytes false
#define __E_BPF_FUNC_l3_csum_replace false
#define __E_BPF_FUNC_l4_csum_replace false
#define __E_BPF_FUNC_tail_call false
#define __E_BPF_FUNC_clone_redirect false
#define __E_BPF_FUNC_get_current_pid_tgid false
#define __E_BPF_FUNC_get_current_uid_gid false
#define __E_BPF_FUNC_get_current_comm false
#define __E_BPF_FUNC_get_cgroup_classid false
#define __E_BPF_FUNC_skb_vlan_push false
#define __E_BPF_FUNC_skb_vlan_pop false
#define __E_BPF_FUNC_skb_get_tunnel_key false
#define __E_BPF_FUNC_skb_set_tunnel_key false
#define __E_BPF_FUNC_perf_event_read false
#define __E_BPF_FUNC_redirect false
#define __E_BPF_FUNC_get_route_realm false
#define __E_BPF_FUNC_perf_event_output false
#define __E_BPF_FUNC_skb_load_bytes false
#define __E_BPF_FUNC_get_stackid false
#define __E_BPF_FUNC_csum_diff false
#define __E_BPF_FUNC_skb_get_tunnel_opt false
#define __E_BPF_FUNC_skb_set_tunnel_opt false
#define __E_BPF_FUNC_skb_change_proto false
#define __E_BPF_FUNC_skb_change_type false
#define __E_BPF_FUNC_skb_under_cgroup false
#define __E_BPF_FUNC_get_hash_recalc false
#define __E_BPF_FUNC_get_current_task false
#define __E_BPF_FUNC_probe_write_user false
#define __E_BPF_FUNC_current_task_under_cgroup false
#define __E_BPF_FUNC_skb_change_tail false
#define __E_BPF_FUNC_skb_pull_data false
#define __E_BPF_FUNC_csum_update false
#define __E_BPF_FUNC_set_hash_invalid false

#if LINUX_VERSION_CODE >= KERNEL_VERSION(3, 19, 0)
#undef __E_BPF_FUNC_map_lookup_elem
#define __E_BPF_FUNC_map_lookup_elem true
#undef __E_BPF_FUNC_map_update_elem
#define __E_BPF_FUNC_map_update_elem true
#undef __E_BPF_FUNC_map_delete_elem
#define __E_BPF_FUNC_map_delete_elem true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 1, 0)
#undef __E_BPF_FUNC_probe_read
#define __E_BPF_FUNC_probe_read true
#undef __E_BPF_FUNC_ktime_get_ns
#define __E_BPF_FUNC_ktime_get_ns true
#undef __E_BPF_FUNC_trace_printk
#define __E_BPF_FUNC_trace_printk true
#undef __E_BPF_FUNC_get_prandom_u32
#define __E_BPF_FUNC_get_prandom_u32 true
#undef __E_BPF_FUNC_get_smp_processor_id
#define __E_BPF_FUNC_get_smp_processor_id true
#undef __E_BPF_FUNC_skb_store_bytes
#define __E_BPF_FUNC_skb_store_bytes true
#undef __E_BPF_FUNC_l3_csum_replace
#define __E_BPF_FUNC_l3_csum_replace true
#undef __E_BPF_FUNC_l4_csum_replace
#define __E_BPF_FUNC_l4_csum_replace true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 2, 0)
#undef __E_BPF_FUNC_tail_call
#define __E_BPF_FUNC_tail_call true
#undef __E_BPF_FUNC_clone_redirect
#define __E_BPF_FUNC_clone_redirect true
#undef __E_BPF_FUNC_get_current_pid_tgid
#define __E_BPF_FUNC_get_current_pid_tgid true
#undef __E_BPF_FUNC_get_current_uid_gid
#define __E_BPF_FUNC_get_current_uid_gid true
#undef __E_BPF_FUNC_get_current_comm
#define __E_BPF_FUNC_get_current_comm true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 3, 0)
#undef __E_BPF_FUNC_get_cgroup_classid
#define __E_BPF_FUNC_get_cgroup_classid true
#undef __E_BPF_FUNC_skb_vlan_push
#define __E_BPF_FUNC_skb_vlan_push true
#undef __E_BPF_FUNC_skb_vlan_pop
#define __E_BPF_FUNC_skb_vlan_pop true
#undef __E_BPF_FUNC_skb_get_tunnel_key
#define __E_BPF_FUNC_skb_get_tunnel_key true
#undef __E_BPF_FUNC_skb_set_tunnel_key
#define __E_BPF_FUNC_skb_set_tunnel_key true
#undef __E_BPF_FUNC_perf_event_read
#define __E_BPF_FUNC_perf_event_read true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 4, 0)
#undef __E_BPF_FUNC_redirect
#define __E_BPF_FUNC_redirect true
#undef __E_BPF_FUNC_get_route_realm
#define __E_BPF_FUNC_get_route_realm true
#undef __E_BPF_FUNC_perf_event_output
#define __E_BPF_FUNC_perf_event_output true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 5, 0)
#undef __E_BPF_FUNC_skb_load_bytes
#define __E_BPF_FUNC_skb_load_bytes true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
#undef __E_BPF_FUNC_get_stackid
#define __E_BPF_FUNC_get_stackid true
#undef __E_BPF_FUNC_csum_diff
#define __E_BPF_FUNC_csum_diff true
#undef __E_BPF_FUNC_skb_get_tunnel_opt
#define __E_BPF_FUNC_skb_get_tunnel_opt true
#undef __E_BPF_FUNC_skb_set_tunnel_opt
#define __E_BPF_FUNC_skb_set_tunnel_opt true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 8, 0)
#undef __E_BPF_FUNC_skb_change_proto
#define __E_BPF_FUNC_skb_change_proto true
#undef __E_BPF_FUNC_skb_change_type
#define __E_BPF_FUNC_skb_change_type true
#undef __E_BPF_FUNC_skb_under_cgroup
#define __E_BPF_FUNC_skb_under_cgroup true
#undef __E_BPF_FUNC_get_hash_recalc
#define __E_BPF_FUNC_get_hash_recalc true
#undef __E_BPF_FUNC_get_current_task
#define __E_BPF_FUNC_get_current_task true
#undef __E_BPF_FUNC_probe_write_user
#define __E_BPF_FUNC_probe_write_user true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 9, 0)
#undef __E_BPF_FUNC_current_task_under_cgroup
#define __E_BPF_FUNC_current_task_under_cgroup true
#undef __E_BPF_FUNC_skb_change_tail
#define __E_BPF_FUNC_skb_change_tail true
#undef __E_BPF_FUNC_skb_pull_data
#define __E_BPF_FUNC_skb_pull_data true
#undef __E_BPF_FUNC_csum_update
#define __E_BPF_FUNC_csum_update true
#undef __E_BPF_FUNC_set_hash_invalid
#define __E_BPF_FUNC_set_hash_invalid true
#endif

#endif /* LINUX_VERSION_CODE < KERNEL_VERSION(4, 10, 0) */

#define bpf_helper_exists(x) __E_ ## x
#endif /* defined(COMPILE_RUNTIME) */

#endif /* defined(__BPF_CROSS_COMPILE__) */
