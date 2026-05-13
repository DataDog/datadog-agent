#ifndef _HELPERS_CGROUP_H_
#define _HELPERS_CGROUP_H_

static __attribute__((always_inline)) u64 get_current_cgroup_id() {
    u64 is_pure_cgroupv2_available = 0;
    LOAD_CONSTANT("is_pure_cgroupv2_available", is_pure_cgroupv2_available);
    if (!is_pure_cgroupv2_available) {
        return 0;
    }

    u64 has_current_cgroup_id_helper = 0;
    LOAD_CONSTANT("has_current_cgroup_id_helper", has_current_cgroup_id_helper);
    if (!has_current_cgroup_id_helper) {
        return 0;
    }

    u64 cgroup_id = bpf_get_current_cgroup_id();

    u64 is_cgroup_id_u64 = 0;
    LOAD_CONSTANT("is_cgroup_id_u64", is_cgroup_id_u64);
    if (!is_cgroup_id_u64) {
        cgroup_id = (u32)cgroup_id;
    }

    return cgroup_id;
}

#endif
