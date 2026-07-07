#ifndef _HELPERS_SPAN_GO_H_
#define _HELPERS_SPAN_GO_H_

#include "maps.h"
#include "process.h"
#include "span_otel.h" // for read_thread_pointer()

// --- Go pprof labels reader (for dd-trace-go) ---
// dd-trace-go sets goroutine-level pprof labels:
//   "span id"            -> decimal string of span ID
//   "local root span id" -> decimal string of local root span ID
//
// The chain from eBPF is:
//   thread_pointer + tls_offset -> G (runtime.g)
//   G + m_offset                -> M (runtime.m)
//   M + curg                    -> curg (current user goroutine)
//   curg + labels               -> labels pointer (map or slice)
//
// The fill_span_context_go function is __noinline to give it its own 512-byte
// stack frame, avoiding overflow when inlined into hooks that already have
// large event structs on the stack.

#define GO_LABEL_MAX_KEY_LEN 24
#define GO_LABEL_MAX_VAL_LEN 24
#define GO_MAX_LABELS 10

// Per-CPU scratch buffer for Go label parsing.
// ALL large allocations live here to stay under the 512-byte eBPF stack limit.
struct go_labels_scratch_t {
    char key_buf[GO_LABEL_MAX_KEY_LEN];
    char val_buf[GO_LABEL_MAX_VAL_LEN];
    struct go_string_t pairs[GO_MAX_LABELS * 2];
    struct go_map_bucket_t bucket;
    struct go_slice_t slice;
};

BPF_PERCPU_ARRAY_MAP(go_labels_scratch_gen, struct go_labels_scratch_t, 1)

// Parse the decimal string in s->val_buf to u64.
// Uses explicit array indexing on the struct field so the verifier can prove
// all accesses stay within the map value bounds.
// The loop uses a running flag instead of break to allow full unrolling.
static u64 __attribute__((always_inline)) parse_decimal_val(struct go_labels_scratch_t *s, u64 len) {
    u64 val = 0;
    int done = 0;
    if (len > 20) len = 20;
    #pragma unroll
    for (int i = 0; i < 20; i++) {
        if (!done && i < (int)len) {
            char c = s->val_buf[i];
            if (c >= '0' && c <= '9') {
                val = val * 10 + (c - '0');
            } else {
                done = 1;
            }
        }
    }
    return val;
}

static void __attribute__((always_inline)) process_go_label(
    struct span_context_t *span,
    struct go_labels_scratch_t *s,
    u64 key_len, u64 val_len)
{
    if (key_len == 7 &&
        s->key_buf[0] == 's' && s->key_buf[1] == 'p' && s->key_buf[2] == 'a' &&
        s->key_buf[3] == 'n' && s->key_buf[4] == ' ' && s->key_buf[5] == 'i' &&
        s->key_buf[6] == 'd') {
        span->span_id = parse_decimal_val(s, val_len);
        return;
    }
    // "local root span id" = 18 chars: l(0)o(1)c(2)a(3)l(4) (5)r(6)o(7)o(8)t(9) (10)s(11)p(12)a(13)n(14) (15)i(16)d(17)
    if (key_len == 18 &&
        s->key_buf[0] == 'l' && s->key_buf[1] == 'o' && s->key_buf[2] == 'c' &&
        s->key_buf[3] == 'a' && s->key_buf[4] == 'l' && s->key_buf[5] == ' ' &&
        s->key_buf[6] == 'r' && s->key_buf[7] == 'o' && s->key_buf[8] == 'o' &&
        s->key_buf[9] == 't' && s->key_buf[10] == ' ' && s->key_buf[11] == 's' &&
        s->key_buf[12] == 'p' && s->key_buf[13] == 'a' && s->key_buf[14] == 'n' &&
        s->key_buf[15] == ' ' && s->key_buf[16] == 'i' && s->key_buf[17] == 'd') {
        span->trace_id[0] = parse_decimal_val(s, val_len);
        return;
    }
}

static int __attribute__((always_inline)) read_and_process_label(
    struct span_context_t *span,
    struct go_labels_scratch_t *s,
    struct go_string_t *key_hdr,
    struct go_string_t *val_hdr)
{
    if (key_hdr->str == NULL || key_hdr->len == 0) {
        return 0;
    }

    __builtin_memset(s->key_buf, 0, GO_LABEL_MAX_KEY_LEN);
    __builtin_memset(s->val_buf, 0, GO_LABEL_MAX_VAL_LEN);

    u64 klen = key_hdr->len;
    if (klen > GO_LABEL_MAX_KEY_LEN) klen = GO_LABEL_MAX_KEY_LEN;
    if (bpf_probe_read_user(s->key_buf, klen & 0x1f, key_hdr->str) < 0) {
        return -1;
    }

    u64 vlen = val_hdr->len;
    if (vlen > GO_LABEL_MAX_VAL_LEN) vlen = GO_LABEL_MAX_VAL_LEN;
    if (vlen > 0 && val_hdr->str != NULL) {
        if (bpf_probe_read_user(s->val_buf, vlen & 0x1f, val_hdr->str) < 0) {
            return -1;
        }
    }

    process_go_label(span, s, key_hdr->len, val_hdr->len);
    return 0;
}

// Try to fill span context from Go pprof labels.
// Returns 1 on success, 0 otherwise.
//
// __noinline: this function gets its own 512-byte stack frame so it doesn't
// add to the calling hook's stack usage. BPF subprograms are supported on
// kernel 5.10+ which is within our target range (5.15+).
int __attribute__((__noinline__)) fill_span_context_go(struct span_context_t *span) {
    if (!span) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct go_labels_offsets_t *offs = bpf_map_lookup_elem(&go_labels_procs, &tgid);
    if (!offs) {
        return 0;
    }

    u32 zero = 0;
    struct go_labels_scratch_t *scratch = bpf_map_lookup_elem(&go_labels_scratch_gen, &zero);
    if (!scratch) {
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

    // Go >=1.24: slice format (hmap_buckets == 0)
    if (offs->hmap_buckets == 0) {
        if (bpf_probe_read_user(&scratch->slice, sizeof(scratch->slice), labels_ptr) < 0) {
            return 0;
        }
        if (scratch->slice.len == 0 || scratch->slice.array == NULL) {
            return 0;
        }
        u64 num_pairs = scratch->slice.len;
        if (num_pairs > GO_MAX_LABELS) num_pairs = GO_MAX_LABELS;

// CADR arm64/6.6 verifier fix: fully unroll with compile-time-constant indices
// so every scratch->pairs[] access is a fixed map-value offset the verifier can
// prove in-bounds. Each pair is read with a constant size (no variable-size read).
#define PROCESS_GO_SLICE_PAIR(N)                                                        \
    do {                                                                                \
        if (num_pairs > (N)) {                                                          \
            const u64 pair_off = (u64)(N) * 2 * sizeof(struct go_string_t);             \
            if (bpf_probe_read_user(&scratch->pairs[(N) * 2],                           \
                                    2 * sizeof(struct go_string_t),                     \
                                    (void *)((char *)scratch->slice.array + pair_off)) < 0) { \
                return 0;                                                               \
            }                                                                           \
            read_and_process_label(span, scratch,                                       \
                                   &scratch->pairs[(N) * 2],                            \
                                   &scratch->pairs[(N) * 2 + 1]);                       \
        }                                                                               \
    } while (0)
        PROCESS_GO_SLICE_PAIR(0);
        PROCESS_GO_SLICE_PAIR(1);
        PROCESS_GO_SLICE_PAIR(2);
        PROCESS_GO_SLICE_PAIR(3);
        PROCESS_GO_SLICE_PAIR(4);
        PROCESS_GO_SLICE_PAIR(5);
        PROCESS_GO_SLICE_PAIR(6);
        PROCESS_GO_SLICE_PAIR(7);
        PROCESS_GO_SLICE_PAIR(8);
        PROCESS_GO_SLICE_PAIR(9);
#undef PROCESS_GO_SLICE_PAIR
        return (span->span_id != 0) ? 1 : 0;
    }

    // Go <1.24: map format
    void *labels_map_ptr = NULL;
    if (bpf_probe_read_user(&labels_map_ptr, sizeof(labels_map_ptr), labels_ptr) < 0 || labels_map_ptr == NULL) {
        return 0;
    }

    u64 labels_count = 0;
    if (bpf_probe_read_user(&labels_count, sizeof(labels_count),
                            labels_map_ptr + offs->hmap_count) < 0 || labels_count == 0) {
        return 0;
    }

    unsigned char log_2_bucket_count = 0;
    if (bpf_probe_read_user(&log_2_bucket_count, sizeof(log_2_bucket_count),
                            labels_map_ptr + offs->hmap_log2_bucket_count) < 0) {
        return 0;
    }

    void *label_buckets = NULL;
    if (bpf_probe_read_user(&label_buckets, sizeof(label_buckets),
                            labels_map_ptr + offs->hmap_buckets) < 0 || label_buckets == NULL) {
        return 0;
    }

    u8 bucket_count = 1 << log_2_bucket_count;
    if (bucket_count > 4) bucket_count = 4;

#define PROCESS_GO_BUCKET_SLOT(N)                                                      \
    do {                                                                               \
        if (scratch->bucket.tophash[(N)] != 0) {                                       \
            read_and_process_label(span, scratch,                                      \
                                   &scratch->bucket.keys[(N)],                         \
                                   &scratch->bucket.values[(N)]);                      \
        }                                                                              \
    } while (0)
#define PROCESS_GO_BUCKET(B)                                                           \
    do {                                                                               \
        if (bucket_count > (B)) {                                                      \
            if (bpf_probe_read_user(&scratch->bucket, sizeof(struct go_map_bucket_t),  \
                                    (void *)((char *)label_buckets + (u64)(B) * sizeof(struct go_map_bucket_t))) < 0) { \
                return 0;                                                              \
            }                                                                          \
            PROCESS_GO_BUCKET_SLOT(0);                                                 \
            PROCESS_GO_BUCKET_SLOT(1);                                                 \
            PROCESS_GO_BUCKET_SLOT(2);                                                 \
            PROCESS_GO_BUCKET_SLOT(3);                                                 \
            PROCESS_GO_BUCKET_SLOT(4);                                                 \
            PROCESS_GO_BUCKET_SLOT(5);                                                 \
            PROCESS_GO_BUCKET_SLOT(6);                                                 \
            PROCESS_GO_BUCKET_SLOT(7);                                                 \
        }                                                                              \
    } while (0)
    PROCESS_GO_BUCKET(0);
    PROCESS_GO_BUCKET(1);
    PROCESS_GO_BUCKET(2);
    PROCESS_GO_BUCKET(3);
#undef PROCESS_GO_BUCKET
#undef PROCESS_GO_BUCKET_SLOT

    return (span->span_id != 0) ? 1 : 0;
}

int __attribute__((always_inline)) unregister_go_labels() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    bpf_map_delete_elem(&go_labels_procs, &tgid);
    return 0;
}

#endif
