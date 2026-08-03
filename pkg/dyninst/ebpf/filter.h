#ifndef __FILTER_H__
#define __FILTER_H__

// BPF runtime helpers for the deferred filter() collection operator. The
// filter loop runs as the enqueue_pc of a per-call-site
// GoFilteredSliceDataType / GoFilteredMapDataType: sm_chase_pointer's
// FILTER_DEFERRED arm jumps to the type's enqueue_pc without writing a
// header or reading source contents; the helpers below drive the
// iterate-and-emit loop from inside that enqueue_pc.
//
// State lives in its own per-CPU map (filter_loop_state_buf, declared in
// context.h) so its presence doesn't grow stack_machine_t and push
// existing field offsets out of the verifier's bounds-tracking window.
// Each helper that touches the state calls filter_loop_state_load() to
// re-derive the pointer with fresh verifier bounds.
//
// Bounds-check pattern. The BPF compiler frequently spills `tail` between
// successive helper calls and the verifier loses range refinement on
// reload, so each `bpf_probe_read_user` here is preceded by a fresh
// scratch_buf_bounds_check with a compile-time-constant `len` (variable
// `len` defeats verifier propagation). For map emit/advance, two writes
// are issued per pair (key at +0, value at +vo), so the constant covers
// the worst-case verifier-visible extent of both fields: with `vo`/`vs`
// each masked to `0xff` (verifier sees each ≤ 255), the worst case is
// sizeof(header) + 2 * MAX_ELEM_BYTES. The runtime `vo + vs ≤
// MAX_ELEM_BYTES` check below is what bounds actual writes; the * 2 is
// the price of the verifier losing that runtime constraint at stack
// spills.

#include "context.h"
#include "framing.h"
#include "scratch.h"
#include "types.h"

// sm_emit_filter_slice_element_noctx emits one per-element data item
// for a slice filter. Re-derives map pointers internally (see
// sm_filter_slice_init_noctx for rationale).
__attribute__((noinline)) int
sm_emit_filter_slice_element_noctx(void) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!buf || !sm || !fst) return 0;
  uint32_t elem_size = fst->elem_size;
  if (elem_size == 0 || elem_size > COLLECTION_PREDICATE_MAX_ELEM_BYTES) {
    return 0;
  }
  target_ptr_t src = fst->data_ptr;
  uint64_t idx = fst->output_index;
  di_data_item_header_t hdr = {
      .type = fst->data_type_id,
      .length = elem_size,
      .address = idx,
  };
  buf_offset_t pre_emit_len = scratch_buf_len(buf);
  buf_offset_t r = scratch_buf_serialize_with_src_256(
      buf, &hdr, src, elem_size);
  if (r == 0) {
    if (!scratch_buf_flush_and_continue(
            buf, &sm->continuation_seq, &sm->last_submitted_seq,
            sm->start_ns, sm->entry_ktime_ns)) {
      sm->continuation_aborted = true;
      return 0;
    }
    pre_emit_len = scratch_buf_len(buf);
    hdr.length = elem_size;
    hdr.address = idx;
    r = scratch_buf_serialize_with_src_256(buf, &hdr, src, elem_size);
    if (r == 0) {
      return 0;
    }
  }
  if (r & FAILED_READ_OFFSET_BIT) {
    scratch_buf_set_len(buf, pre_emit_len);
    return 0;
  }
  // Point sm->offset at the emitted payload so the subsequent CallOp
  // for the element's type handler (ProcessType[T]) reads from the
  // correct buffer location when chasing nested pointers.
  sm->offset = r;
  fst->output_index = idx + 1;
  return 1;
}

// sm_emit_filter_map_element_noctx emits one per-(k,v)-pair data item
// for a map filter. Reads key and value separately from swiss-map slot.
// Re-derives map pointers internally (see sm_filter_slice_init_noctx).
__attribute__((noinline)) int
sm_emit_filter_map_element_noctx(void) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!buf || !sm || !fst) return 0;
  uint32_t ks = fst->elem_size & 0xff;
  uint32_t vs = fst->val_size & 0xff;
  uint32_t vo = fst->val_offset_in_pair & 0xff;
  if (ks == 0) ks = 1;
  if (vs == 0) vs = 1;
  uint32_t payload_len = vo + vs;
  if (payload_len > COLLECTION_PREDICATE_MAX_ELEM_BYTES) return 0;
  uint64_t idx = fst->output_index;
  type_t type = fst->data_type_id;

  target_ptr_t slot_base = fst->map_groups_data +
                           (target_ptr_t)fst->map_group_idx * fst->map_group_byte_size +
                           fst->map_slots_offset +
                           (target_ptr_t)fst->map_slot_idx * fst->map_slot_size;

  // Slack = header + worst-case (vo + vs) per the file-header comment.
  // On failure flush-and-continue so a large matching set spans multiple
  // fragments (mirrors the slice emit path).
  buf_offset_t tail = scratch_buf_len(buf);
  if (!scratch_buf_bounds_check(&tail,
          sizeof(di_data_item_header_t) +
          2 * COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
    if (!scratch_buf_flush_and_continue(
            buf, &sm->continuation_seq, &sm->last_submitted_seq,
            sm->start_ns, sm->entry_ktime_ns)) {
      sm->continuation_aborted = true;
      return 0;
    }
    tail = scratch_buf_len(buf);
    if (!scratch_buf_bounds_check(&tail,
            sizeof(di_data_item_header_t) +
            2 * COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
      return 0;
    }
  }
  di_data_item_header_t hdr = {
      .type = type,
      .length = payload_len,
      .address = idx,
  };
  // Stash original tail on filter_loop_state so we can mutate `tail` in
  // place below without paying for an extra stack local (we're tight
  // against the BPF 512-byte combined-stack limit).
  fst->it_start = tail;
  *(di_data_item_header_t*)(&(*buf)[tail]) = hdr;
  tail += sizeof(di_data_item_header_t);
  if (!scratch_buf_bounds_check(&tail, COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
    return 0;
  }
  uint16_t kios = fst->map_key_in_slot_offset & 0xfff;
  if (bpf_probe_read_user(&(*buf)[tail], ks,
                          (void*)(slot_base + kios)) != 0) {
    return 0;
  }
  tail += vo;
  if (!scratch_buf_bounds_check(&tail, COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
    return 0;
  }
  uint16_t vios = fst->map_val_in_slot_offset & 0xfff;
  if (bpf_probe_read_user(&(*buf)[tail], vs,
                          (void*)(slot_base + vios)) != 0) {
    return 0;
  }

  uint32_t total = sizeof(di_data_item_header_t) + payload_len;
  uint32_t rem = payload_len % 8;
  if (rem != 0) total += 8 - rem;
  scratch_buf_set_len(buf, fst->it_start + total);

  // Point sm->offset at the emitted payload so the subsequent CallOp
  // for the element's type handler reads from the correct location.
  sm->offset = fst->it_start + sizeof(di_data_item_header_t);
  fst->output_index = idx + 1;
  return 1;
}

// sm_emit_filter_slice_marker_noctx: noinline body for
// SM_OP_EMIT_FILTER_SLICE_MARKER. Validates the slice header at
// sm->offset, computes the capped byte length, and stores the chase
// parameters in filter_loop_state (data_type_id, data_ptr,
// remaining as a temporary chase-bytes channel). Returns:
//   0 = chase needed; sm_loop should call sm_record_pointer.
//   1 = skip (null pointer, zero len, or zero elem_size).
//   2 = error (bounds check failure, map lookup failure).
__attribute__((noinline)) int
sm_emit_filter_slice_marker_noctx(type_t data_type, uint32_t elem_size) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!buf || !sm || !fst) return 2;
  if (!scratch_buf_bounds_check(&sm->offset, 24)) {
    return 2;
  }
  target_ptr_t data_ptr = *(target_ptr_t*)(&(*buf)[sm->offset]);
  int64_t signed_len = *(int64_t*)(&(*buf)[sm->offset + 8]);

  if (data_ptr == 0 || signed_len <= 0 || elem_size == 0) {
    return 1;
  }
  uint64_t len = (uint64_t)signed_len;

  if (len > COLLECTION_PREDICATE_MAX_ITERATIONS) {
    sm->pending_expr_status = EXPR_STATUS_TRUNCATED;
    len = COLLECTION_PREDICATE_MAX_ITERATIONS;
  }
  uint64_t capped_bytes = len * (uint64_t)elem_size;
  if (capped_bytes > 0xFFFFFFFFULL) {
    capped_bytes = 0xFFFFFFFFULL;
  }
  // Store chase parameters in sm->di_0 for sm_loop to pass to
  // sm_record_pointer. di_0 is the standard channel for passing data
  // items in the chase path; we reuse it here since the marker runs
  // before any chase processing.
  sm->di_0.type = data_type;
  sm->di_0.address = data_ptr;
  sm->di_0.length = (uint32_t)capped_bytes;
  return 0;
}

// sm_emit_filter_map_marker_noctx: noinline body for
// SM_OP_EMIT_FILTER_MAP_MARKER. Validates the map pointer at
// sm->offset, reads the used count, and stores chase parameters in
// sm->di_0. Returns same codes as the slice variant.
__attribute__((noinline)) int
sm_emit_filter_map_marker_noctx(type_t data_type,
                                uint32_t swiss_header_size,
                                uint32_t used_field_offset) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  if (!buf || !sm) return 2;
  if (!scratch_buf_bounds_check(&sm->offset, 8)) {
    return 2;
  }
  target_ptr_t map_addr = *(target_ptr_t*)(&(*buf)[sm->offset]);
  if (map_addr == 0) {
    return 1;
  }

  uint64_t used = 0;
  if (bpf_probe_read_user(&used, sizeof(used),
                          (void*)(map_addr + used_field_offset)) == 0) {
    if (used > COLLECTION_PREDICATE_MAX_ITERATIONS) {
      sm->pending_expr_status = EXPR_STATUS_TRUNCATED;
    }
  }

  sm->di_0.type = data_type;
  sm->di_0.address = map_addr;
  sm->di_0.length = swiss_header_size;
  return 0;
}

// sm_filter_slice_init_noctx is the SM_OP_INIT_FILTER_SLICE_LOOP body.
//
// Takes no global_ctx_t* by design. Passing a stack pointer to a
// noinline function makes the verifier conservatively assume the callee
// can clobber any caller-stack slot reachable through it, which
// invalidates the typed-pointer tracking on spilled map_value pointers
// in sm_loop and breaks downstream accesses. Every map pointer this
// helper needs is re-derived via bpf_map_lookup_elem so the verifier
// sees fresh, well-typed pointers.
__attribute__((noinline)) int
sm_filter_slice_init_noctx(uint32_t elem_size,
                           uint32_t iter_scratch_budget) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!buf || !sm || !fst) return 2;
  target_ptr_t data_ptr = sm->di_0.address;
  uint32_t total_bytes = sm->di_0.length;
  if (elem_size == 0 || elem_size > COLLECTION_PREDICATE_MAX_ELEM_BYTES) {
    return 2;
  }
  uint32_t remaining = total_bytes / elem_size;
  if (remaining > COLLECTION_PREDICATE_MAX_ITERATIONS) {
    remaining = COLLECTION_PREDICATE_MAX_ITERATIONS;
  }
  fst->data_ptr = data_ptr;
  fst->remaining = remaining;
  fst->elem_size = elem_size;
  fst->data_type_id = sm->di_0.type;
  fst->output_index = 0;
  fst->in_progress = 1;
  fst->is_map = 0;

  if (remaining == 0) {
    return 1;
  }
  buf_offset_t tail = scratch_buf_len(buf);
  if (!scratch_buf_bounds_check(&tail, iter_scratch_budget)) {
    if (!scratch_buf_flush_and_continue(
            buf, &sm->continuation_seq, &sm->last_submitted_seq,
            sm->start_ns, sm->entry_ktime_ns)) {
      sm->continuation_aborted = true;
      return 2;
    }
    tail = scratch_buf_len(buf);
    if (!scratch_buf_bounds_check(&tail, iter_scratch_budget)) {
      return 2;
    }
  }
  fst->it_start = tail;
  sm->cur_loop_it_start = tail;
  uint32_t es = elem_size & (COLLECTION_PREDICATE_MAX_ELEM_BYTES - 1);
  if (es == 0) es = 1;
  if (!scratch_buf_bounds_check(&tail, COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
    fst->in_progress = 0;
    return 2;
  }
  if (bpf_probe_read_user(&(*buf)[tail], es, (void*)data_ptr) != 0) {
    fst->in_progress = 0;
    return 2;
  }
  sm->offset = tail;
  return 0;
}

// sm_filter_slice_advance: SM_OP_FILTER_SLICE_ADVANCE body.
//
// Does not take global_ctx_t* by design (see sm_filter_slice_init_noctx).
__attribute__((noinline)) int
sm_filter_slice_advance_noctx(uint32_t elem_size,
                              uint32_t iter_scratch_budget) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!buf || !sm || !fst) return 2;
  if (elem_size == 0 || elem_size > COLLECTION_PREDICATE_MAX_ELEM_BYTES) {
    return 2;
  }
  fst->data_ptr += elem_size;
  if (fst->remaining > 0) {
    fst->remaining--;
  }
  if (fst->remaining == 0) {
    fst->in_progress = 0;
    return 1;
  }
  buf_offset_t tail = scratch_buf_len(buf);
  if (!scratch_buf_bounds_check(&tail, iter_scratch_budget)) {
    if (!scratch_buf_flush_and_continue(
            buf, &sm->continuation_seq, &sm->last_submitted_seq,
            sm->start_ns, sm->entry_ktime_ns)) {
      sm->continuation_aborted = true;
      return 2;
    }
    tail = scratch_buf_len(buf);
    if (!scratch_buf_bounds_check(&tail, iter_scratch_budget)) {
      return 2;
    }
  }
  fst->it_start = tail;
  sm->cur_loop_it_start = tail;
  uint32_t es = elem_size & (COLLECTION_PREDICATE_MAX_ELEM_BYTES - 1);
  if (es == 0) es = 1;
  if (!scratch_buf_bounds_check(&tail, COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
    fst->in_progress = 0;
    return 2;
  }
  if (bpf_probe_read_user(&(*buf)[tail], es,
                          (void*)fst->data_ptr) != 0) {
    fst->in_progress = 0;
    return 2;
  }
  sm->offset = tail;
  return 0;
}

// sm_filter_map_advance_and_read_noctx: cursor advance + slot read for
// maps. Re-derives map pointers internally (see sm_filter_slice_init_noctx).
__attribute__((noinline)) int
sm_filter_map_advance_and_read_noctx(uint32_t iter_scratch_budget) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!buf || !sm || !fst) return 2;

  if (fst->remaining == 0) {
    fst->in_progress = 0;
    return 1;
  }

  // If the previous call returned a slot to predicate/emit, advance
  // past it now (we don't advance at the return site so that emit can
  // re-read the same slot from user memory).
  if (fst->map_slot_returned) {
    fst->map_slot_returned = 0;
    if (fst->map_slot_idx < 7) {
      fst->map_slot_idx++;
    } else {
      fst->map_slot_idx = 0;
      fst->map_group_idx++;
    }
  }

  #pragma unroll
  for (int scan_step = 0; scan_step < 64; scan_step++) {
    if (fst->map_table_idx >= fst->map_dir_len) {
      fst->in_progress = 0;
      return 1;
    }
    if (!fst->map_loaded_table) {
      if (bpf_probe_read_user(&fst->map_tmp_table_ptr, 8,
                              (void*)(fst->map_dir_ptr + (target_ptr_t)fst->map_table_idx * 8)) != 0) {
        fst->in_progress = 0;
        return 2;
      }
      if (fst->map_tmp_table_ptr == 0 || fst->map_tmp_table_ptr == fst->map_prev_table_ptr) {
        fst->map_table_idx++;
        fst->map_group_idx = 0;
        fst->map_slot_idx = 0;
        continue;
      }
      fst->map_prev_table_ptr = fst->map_tmp_table_ptr;
      target_ptr_t tab = fst->map_tmp_table_ptr + fst->map_table_groups_field_offset;
      if (bpf_probe_read_user(&fst->map_groups_data, 8,
                              (void*)(tab + fst->map_groups_data_field_offset)) != 0) {
        fst->in_progress = 0;
        return 2;
      }
      if (bpf_probe_read_user(&fst->map_length_mask, 8,
                              (void*)(tab + fst->map_groups_len_mask_field_offset)) != 0) {
        fst->in_progress = 0;
        return 2;
      }
      fst->map_loaded_table = 1;
      fst->map_group_idx = 0;
      fst->map_slot_idx = 0;
      continue;
    }
    if ((uint64_t)fst->map_group_idx > fst->map_length_mask) {
      fst->map_table_idx++;
      fst->map_loaded_table = 0;
      continue;
    }
    if (fst->map_slot_idx == 0) {
      if (bpf_probe_read_user(&fst->map_ctrl, 8,
                              (void*)(fst->map_groups_data +
                                      (target_ptr_t)fst->map_group_idx * fst->map_group_byte_size +
                                      fst->map_ctrl_offset)) != 0) {
        fst->in_progress = 0;
        return 2;
      }
    }
    bool found_slot = false;
    uint8_t found_idx = 0;
    #pragma unroll
    for (int s = 0; s < 8; s++) {
      if (s < fst->map_slot_idx) continue;
      if (((fst->map_ctrl >> (s * 8)) & 0x80) == 0) {
        found_slot = true;
        found_idx = (uint8_t)s;
        break;
      }
    }
    if (!found_slot) {
      fst->map_group_idx++;
      fst->map_slot_idx = 0;
      continue;
    }
    fst->map_slot_idx = found_idx;
    buf_offset_t tail = scratch_buf_len(buf);
    if (!scratch_buf_bounds_check(&tail, iter_scratch_budget) ||
        !scratch_buf_bounds_check(&tail, 2 * COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
      if (!scratch_buf_flush_and_continue(
              buf, &sm->continuation_seq, &sm->last_submitted_seq,
              sm->start_ns, sm->entry_ktime_ns)) {
        sm->continuation_aborted = true;
        return 2;
      }
      tail = scratch_buf_len(buf);
      if (!scratch_buf_bounds_check(&tail, iter_scratch_budget) ||
          !scratch_buf_bounds_check(&tail, 2 * COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
        return 2;
      }
    }
    fst->it_start = tail;
    sm->cur_loop_it_start = tail;
    uint32_t ks = fst->elem_size & 0xff;
    uint32_t vs = fst->val_size & 0xff;
    uint32_t vo = fst->val_offset_in_pair & 0xff;
    if (ks == 0) ks = 1;
    if (vs == 0) vs = 1;
    target_ptr_t slot_base = fst->map_groups_data +
                             (target_ptr_t)fst->map_group_idx * fst->map_group_byte_size +
                             fst->map_slots_offset +
                             (target_ptr_t)fst->map_slot_idx * fst->map_slot_size;
    // `tail` is mutated in place between writes (rather than introducing
    // k_off / v_off stack locals) to stay under the BPF 512-byte
    // combined-stack limit. `fst->it_start` (set above) carries the
    // original tail through to sm->offset at the end.
    if (!scratch_buf_bounds_check(&tail, COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
      fst->in_progress = 0;
      return 2;
    }
    uint16_t kios = fst->map_key_in_slot_offset & 0xfff;
    if (bpf_probe_read_user(&(*buf)[tail], ks,
                            (void*)(slot_base + kios)) != 0) {
      fst->in_progress = 0;
      return 2;
    }
    tail += vo;
    if (!scratch_buf_bounds_check(&tail, COLLECTION_PREDICATE_MAX_ELEM_BYTES)) {
      fst->in_progress = 0;
      return 2;
    }
    uint16_t vios = fst->map_val_in_slot_offset & 0xfff;
    if (bpf_probe_read_user(&(*buf)[tail], vs,
                            (void*)(slot_base + vios)) != 0) {
      fst->in_progress = 0;
      return 2;
    }
    // Leave map_slot_idx pointing at the slot we just read; the emit
    // op (when the predicate is true) re-reads the same slot from
    // user memory via slot_base computed from map_slot_idx. Bumping
    // slot_idx here would make emit read the next slot, not the one
    // currently in @it. Advancement happens at the top of the next
    // call via the map_slot_returned flag.
    fst->map_slot_returned = 1;
    fst->remaining--;
    sm->offset = fst->it_start;
    return 0;
  }
  return 3;
}

// sm_filter_map_init_noctx: SM_OP_INIT_FILTER_MAP_LOOP body.
// Re-derives map pointers internally (see sm_filter_slice_init_noctx).
__attribute__((noinline)) int
sm_filter_map_init_noctx(uint32_t iter_scratch_budget) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  filter_loop_state_t* fst = filter_loop_state_load();
  if (!sm || !fst) return 2;

  uint64_t dir_ptr = 0;
  uint64_t dir_len = 0;
  if (bpf_probe_read_user(&dir_ptr, 8,
                          (void*)(sm->di_0.address + fst->map_dir_ptr_offset)) != 0) {
    return 2;
  }
  if (bpf_probe_read_user(&dir_len, 8,
                          (void*)(sm->di_0.address + fst->map_dir_len_offset)) != 0) {
    return 2;
  }
  fst->data_type_id = sm->di_0.type;
  fst->output_index = 0;
  fst->in_progress = 1;
  fst->is_map = 1;
  fst->remaining = COLLECTION_PREDICATE_MAX_ITERATIONS;
  fst->map_dir_ptr = dir_ptr;
  fst->map_dir_len = dir_len;
  fst->map_groups_data = 0;
  fst->map_length_mask = 0;
  fst->map_ctrl = 0;
  fst->map_prev_table_ptr = 0;
  fst->map_tmp_table_ptr = 0;
  fst->map_table_idx = 0;
  fst->map_group_idx = 0;
  fst->map_slot_idx = 0;
  fst->map_loaded_table = 0;
  fst->map_slot_returned = 0;

  if (dir_len == 0) {
    fst->map_dir_len = 1;
    fst->map_groups_data = dir_ptr;
    fst->map_length_mask = 0;
    fst->map_loaded_table = 1;
  }

  return sm_filter_map_advance_and_read_noctx(iter_scratch_budget);
}

#endif // __FILTER_H__
