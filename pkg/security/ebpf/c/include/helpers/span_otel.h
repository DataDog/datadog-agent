#ifndef _HELPERS_SPAN_OTEL_H_
#define _HELPERS_SPAN_OTEL_H_

#include "maps.h"
#include "process.h"

// --- OTel Thread Local Context Record (per OTel spec PR #4947) ---
// Targets native applications using ELF TLS (C, C++, Rust, Java/JNI, etc.).
// Supported architectures: x86_64 (fsbase), ARM64 (tpidr_el0 / uw.tp_value).
// The otel_tls BPF map is populated by user-space after parsing the ELF dynsym table
// for the `otel_thread_ctx_v1` TLS symbol. No eRPC registration is used.
// Go runtime support uses pprof labels instead (see span_go.h).

int __attribute__((always_inline)) unregister_otel_tls() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    bpf_map_delete_elem(&otel_tls, &tgid);

    return 0;
}

// Convert 8 bytes in W3C (big-endian / network byte order) to a native-endian u64.
static u64 __attribute__((always_inline)) otel_bytes_to_u64(const u8 *bytes) {
    return ((u64)bytes[0] << 56) | ((u64)bytes[1] << 48) |
           ((u64)bytes[2] << 40) | ((u64)bytes[3] << 32) |
           ((u64)bytes[4] << 24) | ((u64)bytes[5] << 16) |
           ((u64)bytes[6] << 8)  | ((u64)bytes[7]);
}

// Read the thread pointer (TLS base) from the current task_struct.
// x86_64: task_struct->thread.fsbase
// ARM64:  task_struct->thread.uw.tp_value (tp_value at offset 0 within uw)
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

    u64 tp = 0;
    int ret = bpf_probe_read_kernel(&tp, sizeof(tp),
                                     (void *)task + thread_offset + tp_field_offset);
    if (ret < 0) {
        return 0;
    }
    return tp;
}

// --- TLS resolution (read at probe time) ---
// User-space prepares file-derived metadata: the defining object's load bias,
// STT_TLS symbol offset/size, PT_TLS size, DT_DEBUG address, loader structure
// offsets, DTV layout, and a hash set of PT_TLS modules. eBPF then mirrors the
// tls-modid-bpf sample: walk r_debug.r_map, match the target module's load bias,
// resolve the runtime module ID/static-TLS offset, and read the current thread's
// TLS slot through static TLS or DTV.

#define OTEL_TLS_HASH_MUL1 0xff51afd7ed558ccdULL
#define OTEL_TLS_HASH_MUL2 0xc4ceb9fe1a85ec53ULL

static void __attribute__((always_inline)) set_otel_tls_resolution(
        struct otel_tls_t *otls, u32 status, s32 read_error,
        u64 mod_id, s64 static_tls_offset) {
    otls->resolved = 1;
    otls->status = status;
    otls->resolved_read_error = read_error;
    otls->resolved_mod_id = mod_id;
    otls->resolved_static_tls_offset = static_tls_offset;
}

static int __attribute__((always_inline)) otel_target_symbol_range_in_tls(struct otel_tls_t *otls) {
    u64 tls_memsz = otls->target_tls_memsz;
    u64 offset = otls->target_symbol_offset;
    u64 size = otls->target_symbol_size;

    if (size < sizeof(u64)) {
        return 0;
    }
    if (offset > tls_memsz) {
        return 0;
    }
    if (size > tls_memsz - offset) {
        return 0;
    }
    return 1;
}

static u64 __attribute__((always_inline)) known_otel_tls_module(struct otel_tls_t *otls, u64 load_bias) {
    u64 hash = load_bias >> 12;
    hash ^= otls->tls_module_hash_seed;
    hash ^= hash >> 33;
    hash *= OTEL_TLS_HASH_MUL1;
    hash ^= hash >> 33;
    hash *= OTEL_TLS_HASH_MUL2;
    hash ^= hash >> 33;

    u32 slot = hash & (OTEL_TLS_HASH_SLOTS - 1);
    u64 word = otls->tls_module_hash_bits[slot >> 6];

    return (word >> (slot & 63)) & 1;
}

static int __attribute__((always_inline)) find_otel_r_debug(struct otel_tls_t *otls, u64 *r_debug_addr) {
    if (otls->dt_debug_value_addr == 0) {
        return OTEL_TLS_LOOKUP_NO_R_DEBUG;
    }

    int read_error = bpf_probe_read_user(r_debug_addr, sizeof(*r_debug_addr),
                                         (const void *)otls->dt_debug_value_addr);
    if (read_error) {
        otls->resolved_read_error = read_error;
        return OTEL_TLS_LOOKUP_R_DEBUG_READ_ERROR;
    }
    if (*r_debug_addr == 0) {
        return OTEL_TLS_LOOKUP_NO_R_DEBUG;
    }

    return OTEL_TLS_LOOKUP_OK;
}

static void __attribute__((always_inline)) resolve_otel_static_main(struct otel_tls_t *otls) {
    if (!otel_target_symbol_range_in_tls(otls)) {
        set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_OFFSET_OUT_OF_RANGE, 0, 0, 0);
        return;
    }

    // No-loader static main convention: main module has TLS module ID 1. Keep
    // static_tls_offset at zero so the slot reader uses DTV[1] + STT_TLS st_value.
    set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_OK, 0, 1, 0);
}

static void __attribute__((__noinline__)) resolve_otel_link_map(struct otel_tls_t *otls) {
    u64 r_debug_addr = 0;
    int status = find_otel_r_debug(otls, &r_debug_addr);
    if (status != OTEL_TLS_LOOKUP_OK) {
        set_otel_tls_resolution(otls, status, otls->resolved_read_error, 0, 0);
        return;
    }

    u64 map = 0;
    int read_error = bpf_probe_read_user(&map, sizeof(map),
                                         (const void *)(r_debug_addr + otls->r_debug_r_map_offset));
    if (read_error) {
        set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_R_DEBUG_READ_ERROR, read_error, 0, 0);
        return;
    }

    u64 reconstructed_mod_id = 0;

    for (int i = 0; i < OTEL_TLS_MAX_LINK_MAPS; i++) {
        if (map == 0) {
            set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_NOT_FOUND, 0, 0, 0);
            return;
        }

        u64 real_map = map;
        if (!otls->reconstruct_module_ids && otls->link_map_l_real_offset != 0) {
            read_error = bpf_probe_read_user(&real_map, sizeof(real_map),
                                             (const void *)(map + otls->link_map_l_real_offset));
            if (read_error) {
                set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_READ_ERROR,
                                        read_error, 0, 0);
                return;
            }
            if (real_map == 0) {
                real_map = map;
            }
        }

        u64 load_bias = 0;
        read_error = bpf_probe_read_user(&load_bias, sizeof(load_bias),
                                         (const void *)(real_map + otls->link_map_l_addr_offset));
        if (read_error) {
            set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_READ_ERROR,
                                    read_error, 0, 0);
            return;
        }

        u64 has_tls = 0;
        if (otls->reconstruct_module_ids) {
            has_tls = known_otel_tls_module(otls, load_bias);
            reconstructed_mod_id += has_tls;
        }

        if (load_bias == otls->target_load_bias) {
            if (!otel_target_symbol_range_in_tls(otls)) {
                set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_OFFSET_OUT_OF_RANGE, 0, 0, 0);
                return;
            }

            if (otls->reconstruct_module_ids) {
                if (has_tls == 0) {
                    set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_NO_PT_TLS, 0, 0, 0);
                    return;
                }
                if (reconstructed_mod_id == 0) {
                    set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_NOT_FOUND, 0, 0, 0);
                    return;
                }
                set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_OK, 0, reconstructed_mod_id, 0);
                return;
            }

            u64 mod_id = 0;
            s64 static_tls_offset = 0;
            read_error = bpf_probe_read_user(&mod_id, sizeof(mod_id),
                                             (const void *)(real_map + otls->link_map_l_tls_modid_offset));
            if (read_error) {
                set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_READ_ERROR,
                                        read_error, 0, 0);
                return;
            }
            read_error = bpf_probe_read_user(&static_tls_offset, sizeof(static_tls_offset),
                                             (const void *)(real_map + otls->link_map_l_tls_offset_offset));
            if (read_error) {
                set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_READ_ERROR,
                                        read_error, 0, 0);
                return;
            }

            set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_OK, 0, mod_id, static_tls_offset);
            return;
        }

        u64 next = 0;
        read_error = bpf_probe_read_user(&next, sizeof(next),
                                         (const void *)(map + otls->link_map_l_next_offset));
        if (read_error) {
            set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_READ_ERROR,
                                    read_error, 0, 0);
            return;
        }
        map = next;
    }

    set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_LINK_MAP_NOT_FOUND, 0, 0, 0);
}

static int __attribute__((always_inline)) ensure_otel_tls_resolution(struct otel_tls_t *otls) {
    if (otls->resolved && otls->status == OTEL_TLS_LOOKUP_OK) {
        return OTEL_TLS_LOOKUP_OK;
    }

    otls->resolved_read_error = 0;

    if (otls->mode == OTEL_TLS_MODE_LINK_MAP) {
        resolve_otel_link_map(otls);
    } else if (otls->mode == OTEL_TLS_MODE_STATIC_MAIN) {
        resolve_otel_static_main(otls);
    } else {
        set_otel_tls_resolution(otls, OTEL_TLS_LOOKUP_BAD_MODE, 0, 0, 0);
    }

    return otls->status;
}

static u64 __attribute__((always_inline)) otel_static_tls_block_addr(u64 tp, s64 static_tls_offset) {
#if defined(__aarch64__) || defined(__TARGET_ARCH_arm64)
    return tp + static_tls_offset;
#elif defined(__x86_64__) || defined(__TARGET_ARCH_x86)
    return tp - static_tls_offset;
#else
    return 0;
#endif
}

static int __attribute__((always_inline)) resolve_otel_tls_slot_addr(
        struct otel_tls_t *otls, u64 *tls_slot_addr) {
    u64 tp = read_thread_pointer();
    if (tp == 0) {
        return OTEL_TLS_LOOKUP_NO_THREAD_POINTER;
    }

    if (otls->resolved_static_tls_offset > 0) {
        u64 static_tls_block = otel_static_tls_block_addr(tp, otls->resolved_static_tls_offset);
        if (static_tls_block != 0) {
            *tls_slot_addr = static_tls_block + otls->target_symbol_offset;
            return OTEL_TLS_LOOKUP_OK;
        }
    }

    if (otls->dtv_entry_size == 0 || otls->dtv_entry_size > 64 || otls->resolved_mod_id > OTEL_TLS_MAX_MODULE_ID) {
        return OTEL_TLS_LOOKUP_DTV_READ_ERROR;
    }

    u64 dtv = 0;
    int read_error = bpf_probe_read_user(&dtv, sizeof(dtv),
                                         (const void *)((u64)((s64)tp + otls->tcb_dtv_offset)));
    if (read_error) {
        otls->resolved_read_error = read_error;
        return OTEL_TLS_LOOKUP_DTV_READ_ERROR;
    }

    u64 tls_block = 0;
    read_error = bpf_probe_read_user(&tls_block, sizeof(tls_block),
                                     (const void *)(dtv
                                                    + otls->resolved_mod_id * otls->dtv_entry_size
                                                    + otls->dtv_entry_pointer_offset));
    if (read_error) {
        otls->resolved_read_error = read_error;
        return OTEL_TLS_LOOKUP_DTV_READ_ERROR;
    }
    if (tls_block == 0 || tls_block == ~0ULL) {
        return OTEL_TLS_LOOKUP_TLS_BLOCK_UNAVAILABLE;
    }

    *tls_slot_addr = tls_block + otls->target_symbol_offset;
    return OTEL_TLS_LOOKUP_OK;
}

// Per-CPU scratch buffer for OTel span attributes.
// Avoids placing the 258-byte otel_span_attrs_t on the stack.
BPF_PERCPU_ARRAY_MAP(otel_span_attrs_gen, struct otel_span_attrs_t, 1)
BPF_ARRAY_MAP(otel_attrs_seq, u64, 1)

static u64 __attribute__((always_inline)) next_otel_span_attrs_id(void) {
    u32 zero = 0;
    u64 *seq = bpf_map_lookup_elem(&otel_attrs_seq, &zero);
    if (!seq) {
        return 0;
    }

    u64 next = __sync_fetch_and_add(seq, 1) + 1;
    if (next == 0) {
        next = __sync_fetch_and_add(seq, 1) + 1;
    }

    return next;
}

// Try to fill span context from an OTel Thread Local Context Record.
// Returns 1 on success, 0 otherwise.
// Only attempts TLS resolution for native runtimes (not Go).
int __attribute__((__noinline__)) fill_span_context_otel(struct span_context_t *span) {
    if (!span) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct otel_tls_t *otls = bpf_map_lookup_elem(&otel_tls, &tgid);
    if (!otls) {
        return 0;
    }

    // Only resolve TLS-based context for native runtimes.
    // Go uses pprof labels instead (see span_go.h / fill_span_context_go).
    if (otls->runtime != OTEL_RUNTIME_NATIVE) {
        return 0;
    }

    if (ensure_otel_tls_resolution(otls) != OTEL_TLS_LOOKUP_OK) {
        return 0;
    }

    u64 tls_slot_addr = 0;
    if (resolve_otel_tls_slot_addr(otls, &tls_slot_addr) != OTEL_TLS_LOOKUP_OK || tls_slot_addr == 0) {
        return 0;
    }

    // The TLS variable is a pointer to the active Thread Local Context Record.
    void *record_ptr = NULL;
    int ret = bpf_probe_read_user(&record_ptr, sizeof(record_ptr), (void *)tls_slot_addr);
    if (ret < 0 || record_ptr == NULL) {
        return 0;
    }

    u8 valid_before = 0;
    ret = bpf_probe_read_user(&valid_before, sizeof(valid_before),
                              record_ptr + OTEL_THREAD_CTX_VALID_OFFSET);
    if (ret < 0 || valid_before != 1) {
        return 0;
    }

    // Read the OTel Thread Local Context Record fixed header.
    struct otel_thread_ctx_record_t record = {};
    ret = bpf_probe_read_user(&record, sizeof(record), record_ptr);
    if (ret < 0) {
        return 0;
    }

    u8 valid_after = 0;
    ret = bpf_probe_read_user(&valid_after, sizeof(valid_after),
                              record_ptr + OTEL_THREAD_CTX_VALID_OFFSET);
    if (ret < 0 || record.valid != 1 || valid_after != 1) {
        return 0;
    }

    // Convert W3C byte order (big-endian) to native-endian span_context_t.
    // OTel trace-id: bytes[0..7] = high 64 bits, bytes[8..15] = low 64 bits.
    span->trace_id[1] = otel_bytes_to_u64(&record.trace_id[0]);  // Hi
    span->trace_id[0] = otel_bytes_to_u64(&record.trace_id[8]);  // Lo
    span->span_id = otel_bytes_to_u64(record.span_id);

    // If the record has custom attributes, read them and store in the otel_span_attrs map.
    if (record.attrs_data_size > 0) {
        u32 zero = 0;
        struct otel_span_attrs_t *attrs_val = bpf_map_lookup_elem(&otel_span_attrs_gen, &zero);
        if (attrs_val) {
            u16 attrs_size = record.attrs_data_size;
            if (attrs_size > OTEL_ATTRS_MAX_SIZE) {
                attrs_size = OTEL_ATTRS_MAX_SIZE;
            }

            __builtin_memset(attrs_val, 0, sizeof(*attrs_val));
            attrs_val->size = attrs_size;

            // Read attrs_data from right after the 28-byte fixed header.
            ret = bpf_probe_read_user(attrs_val->data, attrs_size,
                                      record_ptr + sizeof(struct otel_thread_ctx_record_t));
            if (ret >= 0) {
                u64 attrs_id = next_otel_span_attrs_id();
                struct otel_span_attrs_key_t attrs_key = {
                    .id = attrs_id,
                };
                if (attrs_id != 0 && bpf_map_update_elem(&otel_span_attrs, &attrs_key, attrs_val, BPF_ANY) == 0) {
                    span->extra_attrs_id = attrs_id;
                }
            }
        }
    }

    return 1;
}

#endif
