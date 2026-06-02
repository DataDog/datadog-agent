#ifndef __SCRATCH_H__
#define __SCRATCH_H__

#include "compiler.h"
#include "debug.h"
#include "framing.h"
#include "types.h"

// The Linux kernel defines `noinline` as a macro:
//     #define noinline __attribute__((__noinline__))
// (reference: https://elixir.bootlin.com/linux/v5.15/source/include/linux/compiler_attributes.h#L231)
// Our code (via bpf_helpers.h) defines `__noinline` as:
//     #define __noinline __attribute__((noinline))
// If the kernel's `noinline` macro is active during preprocessing,
// it causes `__noinline` to expand into invalid syntax:
//     __attribute__((__attribute__((__noinline__))))
//
// To avoid this conflict, we explicitly undefine `noinline`
#ifdef noinline
#undef noinline
#endif

// Note that this cannot just be uintptr_t because the BPF target has 32-bit
// pointers.
typedef uint64_t target_ptr_t;

typedef uint64_t buf_offset_t;

#define RINGBUF_CAPACITY ((uint64_t)1 << 23)
#define SCRATCH_BUF_LEN ((uint64_t)1 << 15) // 32KiB

// MAX_CONTINUATION_FRAGMENTS caps the number of fragments BPF will emit
// per probe invocation. Once we've submitted this many fragments,
// subsequent flush attempts are rejected and probe_run takes the
// continuation_aborted path (userspace is notified that the event is
// truncated).
//
// 16 fragments × SCRATCH_BUF_LEN = 512 KiB raw payload ceiling per
// invocation. The choice comes from the userspace upload path: snapshots
// are JSON-serialized before upload and we expect at least ~2x size
// blowup from binary-to-JSON encoding (escaping, base64, type tags), and
// the serialized snapshot has a 1 MiB ceiling. Capping raw bytes at
// 512 KiB keeps us comfortably below that limit even for the worst-case
// blowup. This bound exists separately from the per-fragment ringbuf
// pressure check — even a fully drained ringbuf will not accept more
// than this many fragments for a single invocation.
#define MAX_CONTINUATION_FRAGMENTS 16

// Side-channel for drop notifications. Notifications are 32 bytes each; at
// 16 KiB the side channel holds ~500 notifications — far more than the
// number of concurrently-buffered events userspace can hold, so the side
// channel is very unlikely to fill even when out_ringbuf is saturated.
#define DROP_NOTIFY_RINGBUF_CAPACITY ((uint64_t)1 << 14) // 16KiB

struct {
  __uint(type, BPF_MAP_TYPE_RINGBUF);
  __uint(max_entries, RINGBUF_CAPACITY);
} out_ringbuf SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_RINGBUF);
  __uint(max_entries, DROP_NOTIFY_RINGBUF_CAPACITY);
} drop_notify_ringbuf SEC(".maps");

// drop_notify_lost_at records the kernel-monotonic ktime_ns of the most
// recent attempt to publish a drop notification that failed because the
// drop_notify_ringbuf was itself full. Userspace reads this value on its
// periodic stats poll; when it increases, and once the value has been in
// the past for longer than a grace window, userspace evicts buffered
// eventbuf entries whose invocation predates the fault.
//
// Writes are lossy (last-writer wins across CPUs); the semantics we want
// are "some CPU saw a failure at-or-after this time", so racing writes
// simply converge to the latest ktime.
struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, uint64_t);
} drop_notify_lost_at SEC(".maps");

// send_drop_notification publishes a drop notification to the side channel.
// On failure (drop_notify_ringbuf full), stamps drop_notify_lost_at with
// the current ktime so userspace can later reconcile the stuck eventbuf
// entry.
static inline void send_drop_notification(
    uint32_t prog_id,
    uint32_t probe_id,
    uint64_t goid,
    uint32_t stack_byte_depth,
    uint16_t last_seq,
    uint64_t entry_ktime_ns,
    uint8_t drop_reason) {
  di_drop_notification_t notif = {
      .prog_id = prog_id,
      .probe_id = probe_id,
      .goid = goid,
      .stack_byte_depth = stack_byte_depth,
      .drop_reason = drop_reason,
      .last_seq = last_seq,
      .entry_ktime_ns = entry_ktime_ns,
  };
  if (bpf_ringbuf_output(&drop_notify_ringbuf, &notif, sizeof(notif), 0) !=
      0) {
    uint32_t zero = 0;
    uint64_t* slot = bpf_map_lookup_elem(&drop_notify_lost_at, &zero);
    if (slot) {
      *slot = bpf_ktime_get_ns();
    }
  }
}

// A helper to check if the scratch buffer has enough space.
static bool scratch_buf_bounds_check(buf_offset_t* offset, uint64_t len) {
  return *(volatile buf_offset_t*)(offset) <
         (SCRATCH_BUF_LEN - sizeof(di_event_header_t) - len);
}

typedef char scratch_buf_t[SCRATCH_BUF_LEN];

static buf_offset_t scratch_buf_len(scratch_buf_t* scratch_buf) {
  return *((uint32_t*)(scratch_buf));
}

static void scratch_buf_set_len(scratch_buf_t* scratch_buf, uint32_t len) {
  *((uint32_t*)(scratch_buf)) = len;
}

static void scratch_buf_increment_len(scratch_buf_t* scratch_buf,
                                      uint32_t len) {
  *((uint32_t*)(scratch_buf)) += len;
}

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, scratch_buf_t);
} events_scratch_buf_map SEC(".maps");

static di_event_header_t* events_scratch_buf_init(scratch_buf_t** scratch_buf) {
  const uint32_t zero = 0;
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  if (!buf) {
    return NULL;
  }
  *scratch_buf = buf;
  scratch_buf_set_len(*scratch_buf, sizeof(di_event_header_t));
  return (di_event_header_t*)*scratch_buf;
}

static bool events_scratch_buf_submit(scratch_buf_t* scratch_buf,
                                      uint64_t ktime_ns) {
  di_event_header_t* header = (di_event_header_t*)scratch_buf;
  header->ktime_ns = ktime_ns;
  uint64_t len = scratch_buf_len(scratch_buf);
  if (len > SCRATCH_BUF_LEN) {
    len = SCRATCH_BUF_LEN;
  }
  return bpf_ringbuf_output(&out_ringbuf, scratch_buf, len, 0) == 0;
}

// Flush the current scratch buffer as a continuation fragment and reinitialize
// for the next fragment. Returns true on success, false if the flush couldn't
// be submitted — either because the ringbuf is full or because we've reached
// MAX_CONTINUATION_FRAGMENTS. start_ns is the original probe invocation
// timestamp, used to correlate all fragments of a single logical event.
// last_submitted_seq is updated on success so probe_run can fill last_seq on
// any subsequent drop notification.
static bool scratch_buf_flush_and_continue(scratch_buf_t* scratch_buf,
                                           uint16_t* continuation_seq,
                                           uint16_t* last_submitted_seq,
                                           uint64_t start_ns,
                                           uint64_t entry_ktime_ns) {
  // Reject flushes once we'd cross the per-invocation fragment cap. The
  // caller will set continuation_aborted and probe_run will emit the
  // appropriate PARTIAL_* / RETURN_LOST notification — userspace already
  // has fragments [0..MAX_CONTINUATION_FRAGMENTS-1] and is told to
  // finalize them as a truncated event.
  if (*continuation_seq >= MAX_CONTINUATION_FRAGMENTS) {
    LOG(1, "flush: hit MAX_CONTINUATION_FRAGMENTS (%d), aborting continuation",
        MAX_CONTINUATION_FRAGMENTS);
    return false;
  }

  di_event_header_t* header = (di_event_header_t*)scratch_buf;
  header->continuation_seq = *continuation_seq;
  header->continuation_flags = 1; // more fragments follow

  // Save header fields before submit.
  uint32_t prog_id = header->prog_id;
  uint64_t goid = header->goid;
  uint32_t stack_byte_depth = header->stack_byte_depth;
  uint32_t probe_id = header->probe_id;
  unsigned char event_pairing_expectation = header->event_pairing_expectation;

  if (!events_scratch_buf_submit(scratch_buf, start_ns)) {
    return false;
  }

  *last_submitted_seq = *continuation_seq;
  (*continuation_seq)++;

  // Reinitialize the buffer with a continuation header.
  scratch_buf_set_len(scratch_buf, sizeof(di_event_header_t));
  di_event_header_t* new_header = (di_event_header_t*)scratch_buf;
  *new_header = (di_event_header_t){
      .data_byte_len = sizeof(di_event_header_t),
      .prog_id = prog_id,
      .goid = goid,
      .stack_byte_depth = stack_byte_depth,
      .probe_id = probe_id,
      .stack_byte_len = 0, // no stack trace in continuations
      .event_pairing_expectation = event_pairing_expectation,
      .ktime_ns = start_ns, // same timestamp for fragment correlation
      .entry_ktime_ns = entry_ktime_ns,
  };
  return true;
}

typedef struct copy_stack_loop_ctx {
  stack_pcs_t* stack;
  scratch_buf_t* buf;
} copy_stack_loop_ctx_t;

static long copy_stack_loop(unsigned long i, void* _ctx) {
  copy_stack_loop_ctx_t* ctx = (copy_stack_loop_ctx_t*)_ctx;
  if (i >= STACK_DEPTH) {
    return 1;
  }
  target_ptr_t pc = ctx->stack->pcs[i];
  buf_offset_t stack_offset = ((uint64_t)i) * sizeof(target_ptr_t);
  buf_offset_t offset = scratch_buf_len(ctx->buf) + stack_offset;
  if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t))) {
    return 1;
  }
  target_ptr_t* ptr = (target_ptr_t*)(&(*ctx->buf)[offset]);
  *ptr = pc;
  return 0;
}

// This value is used to indicate that the length of the data is not variable
// and that the static length should be used.
#define ENQUEUE_LEN_SENTINEL __UINT32_MAX__

static const uint64_t FAILED_READ_OFFSET_BIT = 1LL << 63;

// Write the queue entry to the scratch buffer, and return the offset of the
// data in the scratch buffer on success or 0 on failure.
static buf_offset_t
scratch_buf_serialize_inner(scratch_buf_t* scratch_buf,
                            di_data_item_header_t* data_item_header,
                            const uint64_t len) {
  buf_offset_t offset = scratch_buf_len(scratch_buf);
  if (!scratch_buf_bounds_check(&offset, sizeof(di_data_item_header_t))) {
    LOG(2, "failed to write data_item_header to scratch buffer %lld", offset);
    return 0;
  }
  // We want to read the length of the data up to the static bound in len.
  // If the length of the data is less than the static bound, then we want
  // to only read that much. This happens when reading variable size data like
  // strings and slices.
  if (data_item_header->length == ENQUEUE_LEN_SENTINEL) {
    data_item_header->length = len;
  }
  uint64_t read_len = data_item_header->length;
  if (read_len >= len) {
    read_len = len;
  }
  data_item_header->length = read_len;
  *(di_data_item_header_t*)(&(*scratch_buf)[offset]) = *data_item_header;
  offset += sizeof(di_data_item_header_t);
  if (!scratch_buf_bounds_check(&offset, len)) {
    LOG(2, "failed to write %d data to scratch buffer %lld", len, offset);
    return 0;
  }
  int read_result = bpf_probe_read_user(&(*scratch_buf)[offset], read_len,
                                        (void*)data_item_header->address);
  scratch_buf_set_len(scratch_buf, offset + read_len);
  int rem = data_item_header->length % 8;
  if (rem != 0) {
    scratch_buf_increment_len(scratch_buf, 8 - rem);
  }
  if (read_result != 0) {
    offset |= FAILED_READ_OFFSET_BIT;
  }
  return offset;
}

// This creates a version of scratch_buf_serialize that can be used with
// a static max_size.
static inline buf_offset_t
scratch_buf_serialize_bounded(scratch_buf_t* scratch_buf,
                              di_data_item_header_t* data_item_header,
                              const uint64_t len, const uint64_t max_size) {
  // Global functions need to check for NULL pointers.
  if (!data_item_header) {
    return 0;
  }
  if (!scratch_buf) {
    return 0;
  }
  if (data_item_header->length == ENQUEUE_LEN_SENTINEL) {
    data_item_header->length = len;
  } else if (data_item_header->length > len) {
    data_item_header->length = len;
  }
  return scratch_buf_serialize_inner(scratch_buf, data_item_header, max_size);
}

#define CONCAT_HELPER(x, y) x##y
#define CONCAT(x, y) CONCAT_HELPER(x, y)

// Use macro magic to define a function for each size class in the list
// of size classes. Then we'll create a single global function which will
// choose the correct size class based on the type info (scratch_buf_serialize).
// Note that the sizes chosen are arbitrary. The first step is relatively large
// so that we almost always hit it. The tradeoff is that even if the pointer we
// want to dereference is only 1 byte, we'd need 1KiB of space left in the
// buffer to get it. That seems like a reasonable tradeoff at this point.
#define SIZE_LIST \
  X(64)           \
  X(256)          \
  X(1024)         \
  X(4096)         \
  X(8192)

// MAX_DATA_ITEM_SIZE is the maximum number of payload bytes the
// scratch_buf_serialize_whole dispatcher can serialize in a single
// data item — i.e., the largest entry in SIZE_LIST. Items larger
// than this can't be serialized in one shot; the BPF stack machine
// clamps serialize_len to this value before calling serialize_whole
// so an oversized maxLength / collection size produces a truncated
// capture rather than a silent skip (see stack_machine.h).
//
// Keep in sync with SIZE_LIST above.
#define MAX_DATA_ITEM_SIZE 8192

#define X(max_size)                                                          \
  buf_offset_t CONCAT(scratch_buf_serialize_, max_size)(                     \
      scratch_buf_t * scratch_buf, di_data_item_header_t * data_item_header, \
      const uint64_t len) {                                                  \
    return scratch_buf_serialize_bounded(scratch_buf, data_item_header, len, \
                                         max_size);                          \
  }

SIZE_LIST

#undef X

buf_offset_t scratch_buf_serialize_whole(scratch_buf_t* scratch_buf,
                                         di_data_item_header_t* data_item_header,
                                         const uint64_t len) {
  // Use macro to also define the checking for the size classes.
#define X(max_size)                                                         \
  if (len <= max_size) {                                                    \
    return CONCAT(scratch_buf_serialize_, max_size)(scratch_buf,            \
                                                    data_item_header, len); \
  }
  SIZE_LIST
#undef X

  return 0;
}

typedef struct read_by_frame_ctx {
  void* addr;
  scratch_buf_t* buf;
  buf_offset_t offset;
  uint64_t len;
  bool buf_out_of_space;
} read_by_frame_ctx_t;

#define DYNINST_PAGE_SIZE 4096

static long read_by_frame_loop(unsigned long i, void* _ctx) {
  read_by_frame_ctx_t* ctx = (read_by_frame_ctx_t*)_ctx;
  if (i * DYNINST_PAGE_SIZE >= ctx->len) {
    return 1;
  }
  buf_offset_t offset = ctx->offset + i * DYNINST_PAGE_SIZE;
  uint64_t len = ctx->len - i * DYNINST_PAGE_SIZE;
  if (len > DYNINST_PAGE_SIZE) {
    len = DYNINST_PAGE_SIZE;
  }
  if (!scratch_buf_bounds_check(&offset, DYNINST_PAGE_SIZE)) {
    ctx->buf_out_of_space = true;
    return 1;
  }
  bpf_probe_read_user(&(*ctx->buf)[offset], len, ctx->addr + i * DYNINST_PAGE_SIZE);
  // We ignore the failure, assuming the object was never accessed before,
  // and thus this fragment is zero'd. bpf_probe_read_user does zero the
  // destination buffer bytes on failure.
  return 0;
}

// This is a fallback for when the first page of the object is not accessible.
// It reads the rest of the object page-by-page, with zero bytes for each
// fragment that failed to read. It assumes that the data item header has
// already been written to the scratch buffer and that data_offset is the offset
// of the first byte of the data in the scratch buffer.
static buf_offset_t
scratch_buf_serialize_fallback(scratch_buf_t* scratch_buf,
                               di_data_item_header_t* data_item_header,
                               buf_offset_t data_offset) {
  // There might be a valid, never fully accessed object. First access to parts
  // of this object trigger a page fault. We assume that first page containing
  // the object should have been accessed, as it should contain non-zero go
  // allocation header. If reading the first page succeeds, we assume this is
  // the case, read rest of the object page-by-page, with zero bytes for each
  // fragment that failed to read.
  uint64_t page_reminder = DYNINST_PAGE_SIZE - (data_item_header->address % DYNINST_PAGE_SIZE);
  if (page_reminder >= data_item_header->length) {
    // Object doesn't cross page.
    return data_offset | FAILED_READ_OFFSET_BIT;
  }
  if (page_reminder >= DYNINST_PAGE_SIZE) {
    return 0;
  }
  if (!scratch_buf_bounds_check(&data_offset, DYNINST_PAGE_SIZE)) {
    return 0;
  }
  int read_result = bpf_probe_read_user(&(*scratch_buf)[data_offset], page_reminder,
                                        (void*)data_item_header->address);
  if (read_result != 0) {
    return data_offset | FAILED_READ_OFFSET_BIT;
  }
  read_by_frame_ctx_t ctx = {
      .addr = (void*)data_item_header->address,
      .buf = scratch_buf,
      .offset = data_offset + page_reminder,
      .len = data_item_header->length - page_reminder,
      .buf_out_of_space = false,
  };
  bpf_loop((ctx.len + DYNINST_PAGE_SIZE - 1) / DYNINST_PAGE_SIZE, read_by_frame_loop, &ctx, 0);
  if (ctx.buf_out_of_space) {
    return 0;
  }
  return data_offset;
}

static buf_offset_t
scratch_buf_serialize_with_fallback(scratch_buf_t* scratch_buf,
                                    di_data_item_header_t* data_item_header,
                                    uint64_t len) {
  buf_offset_t data_offset =
      scratch_buf_serialize_whole(scratch_buf, data_item_header, len);
  if ((data_offset & FAILED_READ_OFFSET_BIT) == 0) {
    return data_offset;
  }
  buf_offset_t base_data_offset = (data_offset & ~FAILED_READ_OFFSET_BIT);
  buf_offset_t fb = scratch_buf_serialize_fallback(
      scratch_buf, data_item_header, base_data_offset);
  if (fb == 0) {
    // Roll back to the state before writing the header.
    scratch_buf_set_len(scratch_buf,
                        base_data_offset - sizeof(di_data_item_header_t));
  }
  return fb;
}

buf_offset_t scratch_buf_serialize(scratch_buf_t* scratch_buf,
                                   di_data_item_header_t* data_item_header,
                                   uint64_t len) {
  if (!scratch_buf) {
    return 0;
  }
  if (!data_item_header) {
    return 0;
  }
  buf_offset_t offset =
      scratch_buf_serialize_with_fallback(scratch_buf, data_item_header, len);
  if ((offset & FAILED_READ_OFFSET_BIT) == 0) {
    LOG(5, "serialized scratch@%lld (!%d [%d]) < user@%llx", offset,
        data_item_header->type, data_item_header->length,
        data_item_header->address);
    return offset;
  }
  LOG(3, "failed to read %lld bytes from %llx",
      data_item_header->length, data_item_header->address);
  offset &= ~FAILED_READ_OFFSET_BIT;
  offset -= sizeof(di_data_item_header_t);
  if (scratch_buf_bounds_check(&offset, sizeof(di_data_item_header_t))) {
    ((di_data_item_header_t*)(&(*scratch_buf)[offset]))->type |= (1 << 31);
  }
  return 0;
}

static bool scratch_buf_dereference_inner(scratch_buf_t* scratch_buf,
                                          buf_offset_t offset, uint64_t len,
                                          const uint64_t max_len,
                                          target_ptr_t ptr) {
  buf_offset_t real_len = len;
  if (real_len > max_len) {
    return false;
  }
  uint64_t real_offset = offset;
  if (!scratch_buf_bounds_check(&real_offset, max_len)) {
    LOG(2, "failed to write %d data to scratch buffer %lld", real_len,
        real_offset);
    return false;
  }
  int read_result =
      bpf_probe_read_user(&(*scratch_buf)[real_offset], real_len, (void*)ptr);

  if (read_result != 0) {
    LOG(3, "failed to read %lld bytes from %llx: %d", real_len, ptr,
        read_result);
    return false;
  };
  LOG(5, "recorded scratch@%lld < user@%llx [%d]", real_offset, ptr, real_len);
  return true;
}

#define X(max_size)                                                          \
  __attribute__((noinline)) bool CONCAT(scratch_buf_dereference_, max_size)( \
      scratch_buf_t * scratch_buf, buf_offset_t offset, uint64_t len,        \
      target_ptr_t ptr) {                                                    \
    if (!scratch_buf) {                                                      \
      return false;                                                          \
    }                                                                        \
    return scratch_buf_dereference_inner(scratch_buf, offset, len, max_size, \
                                         ptr);                               \
  }

SIZE_LIST

#undef X

bool scratch_buf_dereference(scratch_buf_t* scratch_buf, buf_offset_t offset,
                             uint64_t len, target_ptr_t ptr) {
  if (!scratch_buf) {
    return false;
  }
  // Use macro to also define the checking for the size classes.
#define X(max_size)                                                        \
  if (len <= max_size) {                                                   \
    return CONCAT(scratch_buf_dereference_, max_size)(scratch_buf, offset, \
                                                      len, ptr);           \
  }
  SIZE_LIST
#undef X

  return true;
}

// Write a root queue entry to the scratch buffer, and return the offset of
// the data in the scratch buffer on success or 0 on failure. Note that
// nothing will have been written into that data yet; it is expected that the
// caller populate it.
__maybe_unused static buf_offset_t
scratch_buf_reserve(scratch_buf_t* scratch_buf,
                    di_data_item_header_t* data_item_header) {
  if (!scratch_buf) {
    return 0;
  }
  if (!data_item_header) {
    return 0;
  }
  uint32_t padded_len = data_item_header->length;
  uint32_t rem = padded_len % 8;
  if (rem != 0) {
    padded_len += (8 - rem);
  }
  buf_offset_t offset = scratch_buf_len(scratch_buf);
  if (!scratch_buf_bounds_check(&offset,
                                sizeof(di_data_item_header_t) + padded_len)) {
    return 0;
  }
  if (!scratch_buf_bounds_check(&offset, sizeof(di_data_item_header_t))) {
    return 0;
  }
  *(di_data_item_header_t*)(&(*scratch_buf)[offset]) = *data_item_header;
  scratch_buf_increment_len(scratch_buf,
                            sizeof(di_data_item_header_t) + padded_len);
  return offset + sizeof(di_data_item_header_t);
}

#endif // __SCRATCH_H__
