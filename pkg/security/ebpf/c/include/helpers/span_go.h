#ifndef _HELPERS_SPAN_GO_H_
#define _HELPERS_SPAN_GO_H_

#include <assert.h>

#include "maps.h"
#include "process.h"

// Reads the current thread's TLS thread pointer (x86 fsbase / ARM64 tpidr) from
// the kernel task_struct via BTF-resolved offsets. Used as the base for the
// Go runtime.g lookup below.
static u64 __attribute__((always_inline)) read_thread_pointer() {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    u64 thread_offset = get_task_struct_thread_offset();

#if defined(__x86_64__)
    u64 tp_field_offset = get_thread_struct_fsbase_offset();
#elif defined(__aarch64__)
    u64 tp_field_offset = get_thread_struct_uw_offset();
#else
    return 0;
#endif

    // 0 means the offset did not resolved (those fields are never at the start of the struct)
    if (thread_offset == 0 || tp_field_offset == 0) {
        return 0;
    }

    u64 tp = 0;
    int ret = bpf_probe_read_kernel(&tp, sizeof(tp),
                                     (void *)task + thread_offset + tp_field_offset);
    if (ret < 0) {
        return 0;
    }
    return tp;
}

// --- Go pprof labels reader (for dd-trace-go) ---
// dd-trace-go sets goroutine-level pprof labels (e.g. "span id",
// "local root span id"). Rather than interpret them in eBPF, we copy the raw
// key/value bytes into the go_labels_ctx ring and let user space parse them
// (mirrors the syscall_ctx design).
//
// The chain from eBPF is:
//   thread_pointer + tls_offset -> G (runtime.g)
//   G + m_offset                -> M (runtime.m)
//   M + curg                    -> curg (current user goroutine)
//   curg + labels               -> labels pointer (map or slice)

// Mint a monotonically-increasing id for a labels snapshot.
static u32 __attribute__((always_inline)) mint_go_labels_id() {
    u32 key = 0;
    u32 *id = bpf_map_lookup_elem(&go_labels_ctx_gen_id, &key);
    if (!id) {
        return 0;
    }
    __sync_fetch_and_add(id, 1);
    return *id;
}

// Look up the ring slot for a given id (id % GO_LABELS_CTX_MAX_ENTRIES).
static struct go_labels_ctx_entry_t * __attribute__((always_inline)) lookup_go_labels_entry(u32 id) {
    u32 key = id % GO_LABELS_CTX_MAX_ENTRIES;
    return bpf_map_lookup_elem(&go_labels_ctx, &key);
}

// Zero the key/value lengths of every pair slot so a reused ring entry never
// leaks a previous snapshot's labels. Unrolled with constant indices.
static void __attribute__((always_inline)) reset_go_labels_entry(struct go_labels_ctx_entry_t *entry) {
    #pragma unroll
    for (int i = 0; i < GO_LABELS_CTX_MAX_PAIRS; i++) {
        entry->pairs[i].key_len = 0;
        entry->pairs[i].val_len = 0;
    }
}

// Copy one raw key/value pair into entry->pairs[n]. n MUST be a compile-time
// constant so the map-value access is a provable fixed offset.
static int __attribute__((always_inline)) store_go_label(
    struct go_labels_ctx_entry_t *entry, int n,
    struct go_string_t *key_hdr, struct go_string_t *val_hdr)
{
    if (key_hdr->str == NULL || key_hdr->len == 0) {
        return 0;
    }

    __builtin_memset(entry->pairs[n].key, 0, GO_LABELS_CTX_KEY_SIZE);
    __builtin_memset(entry->pairs[n].val, 0, GO_LABELS_CTX_VAL_SIZE);

    // barrier_var() forces klen and vlen to be materialized so the explicit mask
    // survives and the verifier can prove the bound.
    //
    // The `((len - 1) & (SIZE - 1)) + 1` is required for the verifier to prove that len is non zero
    // even after len > 0.
    u64 klen = key_hdr->len;
    if (klen > GO_LABELS_CTX_KEY_SIZE - 1) {
        klen = GO_LABELS_CTX_KEY_SIZE - 1;
    }
    barrier_var(klen);
    klen = ((klen - 1) & (GO_LABELS_CTX_KEY_SIZE - 1)) + 1;

    if (bpf_probe_read_user(entry->pairs[n].key, klen, key_hdr->str) < 0) {
        return 0;
    }

    if (val_hdr->len > 0 && val_hdr->str != NULL) {
        u64 vlen = val_hdr->len;
        if (vlen > GO_LABELS_CTX_VAL_SIZE - 1) {
            vlen = GO_LABELS_CTX_VAL_SIZE - 1;
        }
        barrier_var(vlen);
        vlen = ((vlen - 1) & (GO_LABELS_CTX_VAL_SIZE - 1)) + 1;

        if (bpf_probe_read_user(entry->pairs[n].val, vlen, val_hdr->str) < 0) {
            return 0;
        }
    }

    // Store the real value, userspace clamps it with the buffer
    entry->pairs[n].key_len = key_hdr->len;
    entry->pairs[n].val_len = val_hdr->len;
    return 1;
}

// Go >=1.24: labels are a slice of {key, value} string headers. Read pair n (a
// compile-time constant) and store it into entry->pairs[n].
static void __attribute__((always_inline)) collect_go_slice_pair(
    struct go_labels_ctx_entry_t *entry,
    struct go_labels_scratch_t *s,
    u64 num_pairs, int n)
{
    if (num_pairs <= (u64)n) {
        return;
    }
    const u64 pair_off = (u64)n * 2 * sizeof(struct go_string_t);
    if (bpf_probe_read_user(&s->pairs[n * 2], 2 * sizeof(struct go_string_t),
                            (void *)((char *)s->slice.array + pair_off)) < 0) {
        return;
    }
    store_go_label(entry, n, &s->pairs[n * 2], &s->pairs[n * 2 + 1]);
}

// Go <1.24: labels are a map[string]string. We best-effort read the first
// bucket only (8 slots) into entry->pairs[0..7]; dd-trace-go sets few labels so
// they land in the first bucket. slot MUST be a compile-time constant.
static void __attribute__((always_inline)) collect_go_bucket_slot(
    struct go_labels_ctx_entry_t *entry,
    struct go_labels_scratch_t *s,
    int slot)
{
    if (s->bucket.tophash[slot] != 0) {
        store_go_label(entry, slot, &s->bucket.keys[slot], &s->bucket.values[slot]);
    }
}

// Snapshot the current goroutine's pprof labels into the go_labels_ctx ring.
// Returns the id, 0 when the process is not a tracked Go tracer / no labels are
// available.
static u32 __attribute__((always_inline)) collect_go_labels(void) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct go_labels_offsets_t *offs = bpf_map_lookup_elem(&go_labels_procs, &tgid);
    if (!offs) {
        return 0;
    }

    u64 tp = read_thread_pointer();
    if (tp == 0) {
        return 0;
    }

    // TLS -> G
    u64 g_addr = 0;
    if (bpf_probe_read_user(&g_addr, sizeof(g_addr),
                            (void *)((s64)tp + offs->tls_offset)) < 0 || g_addr == 0) {
        return 0;
    }

    // G -> M
    void *m_ptr = NULL;
    if (bpf_probe_read_user(&m_ptr, sizeof(m_ptr),
                            (void *)(g_addr + offs->m_offset)) < 0 || m_ptr == NULL) {
        return 0;
    }

    // M -> curg
    u64 curg_addr = 0;
    if (bpf_probe_read_user(&curg_addr, sizeof(curg_addr),
                            (void *)((u64)m_ptr + offs->curg)) < 0 || curg_addr == 0) {
        return 0;
    }

    // curg -> labels
    void *labels_ptr = NULL;
    if (bpf_probe_read_user(&labels_ptr, sizeof(labels_ptr),
                            (void *)(curg_addr + offs->labels)) < 0 || labels_ptr == NULL) {
        return 0;
    }

    u32 zero = 0;
    struct go_labels_scratch_t *scratch = bpf_map_lookup_elem(&go_labels_scratch_gen, &zero);
    if (!scratch) {
        return 0;
    }

    // Claim the ring slot before collecting: the collect_* helpers below write
    // the pairs straight into it.
    u32 id = mint_go_labels_id();
    struct go_labels_ctx_entry_t *entry = lookup_go_labels_entry(id);
    if (!entry) {
        return 0;
    }
    reset_go_labels_entry(entry);
    entry->id = id;

    if (offs->hmap_buckets == 0) {
        // Go >=1.24: slice format.
        if (bpf_probe_read_user(&scratch->slice, sizeof(scratch->slice), labels_ptr) < 0) {
            return 0;
        }
        if (scratch->slice.len == 0 || scratch->slice.array == NULL) {
            return 0;
        }
        u64 num_pairs = scratch->slice.len;
        if (num_pairs > GO_LABELS_CTX_MAX_PAIRS) {
            num_pairs = GO_LABELS_CTX_MAX_PAIRS;
        }

        // Manually unrolled over GO_LABELS_CTX_MAX_PAIRS with constant indices.
        static_assert(GO_LABELS_CTX_MAX_PAIRS == 10, "unrolled loop below must match GO_LABELS_CTX_MAX_PAIRS");
        collect_go_slice_pair(entry, scratch, num_pairs, 0);
        collect_go_slice_pair(entry, scratch, num_pairs, 1);
        collect_go_slice_pair(entry, scratch, num_pairs, 2);
        collect_go_slice_pair(entry, scratch, num_pairs, 3);
        collect_go_slice_pair(entry, scratch, num_pairs, 4);
        collect_go_slice_pair(entry, scratch, num_pairs, 5);
        collect_go_slice_pair(entry, scratch, num_pairs, 6);
        collect_go_slice_pair(entry, scratch, num_pairs, 7);
        collect_go_slice_pair(entry, scratch, num_pairs, 8);
        collect_go_slice_pair(entry, scratch, num_pairs, 9);
    } else {
        // Go <1.24: map[string]string format.
        void *labels_map_ptr = NULL;
        if (bpf_probe_read_user(&labels_map_ptr, sizeof(labels_map_ptr), labels_ptr) < 0 || labels_map_ptr == NULL) {
            return 0;
        }

        u64 labels_count = 0;
        if (bpf_probe_read_user(&labels_count, sizeof(labels_count),
                                labels_map_ptr + offs->hmap_count) < 0 || labels_count == 0) {
            return 0;
        }

        void *label_buckets = NULL;
        if (bpf_probe_read_user(&label_buckets, sizeof(label_buckets),
                                labels_map_ptr + offs->hmap_buckets) < 0 || label_buckets == NULL) {
            return 0;
        }

        if (bpf_probe_read_user(&scratch->bucket, sizeof(struct go_map_bucket_t), label_buckets) < 0) {
            return 0;
        }

        // Manually unrolled over GO_MAP_BUCKET_SIZE with constant indices.
        static_assert(GO_MAP_BUCKET_SIZE == 8, "unrolled loop below must match GO_MAP_BUCKET_SIZE");
        collect_go_bucket_slot(entry, scratch, 0);
        collect_go_bucket_slot(entry, scratch, 1);
        collect_go_bucket_slot(entry, scratch, 2);
        collect_go_bucket_slot(entry, scratch, 3);
        collect_go_bucket_slot(entry, scratch, 4);
        collect_go_bucket_slot(entry, scratch, 5);
        collect_go_bucket_slot(entry, scratch, 6);
        collect_go_bucket_slot(entry, scratch, 7);
    }

    return id;
}

static int __attribute__((always_inline)) unregister_go_labels() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    bpf_map_delete_elem(&go_labels_procs, &tgid);
    return 0;
}

#endif
