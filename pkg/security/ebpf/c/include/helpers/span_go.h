#ifndef _HELPERS_SPAN_GO_H_
#define _HELPERS_SPAN_GO_H_

#include "maps.h"
#include "process.h"

#include "thread_pointer.h"

#if defined(__aarch64__)
// Processor state bits used to tell a user-mode register context from a
// kernel-mode one. From arch/arm64/include/uapi/asm/ptrace.h.
#define PSR_MODE_MASK 0x0000000f
#define PSR_MODE_EL0t 0x00000000

// bpf_task_pt_regs() landed in kernel 5.15. The call sites below are guarded by
// this load-time constant so the verifier folds the branch away and never walks
// the (unknown) helper call on older kernels.
static __attribute__((always_inline)) u64 has_task_pt_regs_helper() {
    u64 has_task_pt_regs_helper;
    LOAD_CONSTANT("has_task_pt_regs_helper", has_task_pt_regs_helper);
    return has_task_pt_regs_helper;
}
#endif

// Reads the current goroutine pointer out of the task's saved user registers.
//
// On arm64 the Go ABI keeps g in R28, and a program built without cgo keeps it
// *only* there: runtime.save_g is a no-op when runtime.iscgo is false, so the
// TLS slot is never written and user space reports a tls_offset of 0.
// See https://github.com/golang/go/blob/master/src/runtime/tls_arm64.s.
//
// x86_64 has no dedicated g register, so g always lives in TLS and there is
// nothing to fall back to.
static u64 __attribute__((always_inline)) read_go_g_register() {
#if defined(__aarch64__)
    // bpf_task_pt_regs() requires kernel 5.15; the constant is folded at load
    // time so the verifier never walks the call on older kernels.
    if (!has_task_pt_regs_helper()) {
        return 0;
    }

    // Must be bpf_get_current_task_btf(): bpf_task_pt_regs() only accepts a
    // trusted BTF pointer, and bpf_get_current_task() returns a scalar, which
    // the verifier rejects with "R1 type=scalar expected=ptr_, trusted_ptr_".
    struct task_struct *task = bpf_get_current_task_btf();
    if (task == NULL) {
        return 0;
    }

    struct user_pt_regs *user_regs = (struct user_pt_regs *)bpf_task_pt_regs(task);
    if (user_regs == NULL) {
        return 0;
    }

    // Not every task we run on has a user-mode register context: kernel threads
    // do not, and the network programs below run in softirq on whatever task
    // happens to be current. Only EL0t means the saved state is user mode.
    u64 pstate = 0;
    if (bpf_probe_read_kernel(&pstate, sizeof(pstate), &user_regs->pstate) < 0) {
        return 0;
    }
    if ((pstate & PSR_MODE_MASK) != PSR_MODE_EL0t) {
        return 0;
    }

    u64 g_addr = 0;
    if (bpf_probe_read_kernel(&g_addr, sizeof(g_addr), &user_regs->regs[28]) < 0) {
        return 0;
    }
    return g_addr;
#else
    return 0;
#endif
}

// --- Go pprof labels reader (for dd-trace-go) ---
// dd-trace-go sets goroutine-level pprof labels (e.g. "span id",
// "local root span id"). Rather than interpret them in eBPF, we copy the raw
// key/value bytes into the go_labels_ctx ring and let user space parse them
// (mirrors the syscall_ctx design).
//
// The chain from eBPF is:
//   thread_pointer + tls_offset -> G (runtime.g)   (or R28, see read_go_g_register)
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
    // The return value of __sync_fetch_and_add is deliberately discarded: using
    // it emits BPF_ATOMIC|BPF_FETCH, which older kernels reject.
    __sync_fetch_and_add(id, 1);
    u32 minted = *id;
    if (minted == 0) {
        // The u32 counter wrapped around and 0 is our "invalid id" sentinel,
        // skip it.
        __sync_fetch_and_add(id, 1);
        minted = *id;
    }
    return minted;
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
    if (bpf_probe_read_user(&s->pair, sizeof(s->pair),
                            (void *)((char *)s->slice.array + pair_off)) < 0) {
        return;
    }
    store_go_label(entry, n, &s->pair[0], &s->pair[1]);
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

    // TLS -> G, with a fallback to the g register where the ABI has one. A
    // tls_offset of 0 means user space determined that this binary does not keep
    // g in TLS at all, so the register is the only source.
    u64 g_addr = 0;
    if (offs->tls_offset != 0) {
        u64 tp = read_thread_pointer();
        if (tp != 0 && bpf_probe_read_user(&g_addr, sizeof(g_addr),
                                           (void *)((s64)tp + offs->tls_offset)) < 0) {
            g_addr = 0;
        }
    }
    if (g_addr == 0) {
        g_addr = read_go_g_register();
    }
    if (g_addr == 0) {
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
        _Static_assert(GO_LABELS_CTX_MAX_PAIRS == 10, "unrolled loop below must match GO_LABELS_CTX_MAX_PAIRS");
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
        _Static_assert(GO_MAP_BUCKET_SIZE == 8, "unrolled loop below must match GO_MAP_BUCKET_SIZE");
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
