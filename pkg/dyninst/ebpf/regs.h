#ifndef __REGS_H__
#define __REGS_H__

#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "vmlinux.h"

// Registers are addressed by DWARF register number everywhere in dyninst: the
// compiler reads register numbers straight out of the target's debug info.
//
// x86-64: System V psABI, figure 3.36.
// arm64: "DWARF for the Arm 64-bit Architecture", table 1.
#if defined(bpf_target_x86)

#define DWARF_REGNO_COUNT 17
#define DWARF_REGNO_FP 6
#define DWARF_REGNO_SP 7
// Go keeps the current goroutine pointer in r14 on x86-64.
#define DWARF_REGNO_G 14
#define DWARF_REGNO_PC 16

#elif defined(bpf_target_arm64)

#define DWARF_REGNO_COUNT 33
// Go keeps the current goroutine pointer in x28 on arm64.
#define DWARF_REGNO_G 28
#define DWARF_REGNO_FP 29
#define DWARF_REGNO_LR 30
#define DWARF_REGNO_SP 31
#define DWARF_REGNO_PC 32

#else
#error "Unsupported architecture"
#endif

// dwarf_regs_t is dyninst's own snapshot of a target thread's registers,
// indexed by DWARF register number.
//
// It is deliberately not the kernel's struct pt_regs. struct pt_regs has a
// kernel-internal tail that changes between kernel versions: on arm64 Linux
// 7.0 dropped two fields from it, shrinking it from 336 to 320 bytes. The
// verifier bounds reads of a probe's context by the running kernel's
// sizeof(struct pt_regs), so a program compiled against a bigger layout is
// rejected ("invalid bpf_context access") the moment it touches the tail -
// which copying the whole struct out of the context does. Snapshotting only
// the registers we name keeps every context read inside the part of the
// layout that the architecture's user-space ABI fixes forever.
typedef struct dwarf_regs {
  uint64_t regs[DWARF_REGNO_COUNT];
} dwarf_regs_t;

// Size of the leading part of a probe's context whose layout is fixed by the
// architecture's user-space ABI, and so is the same on every kernel.
//
// The sizes are spelled out as literals on purpose: they are ABI and must
// never change, so a vmlinux.h update that moves them should break the build
// here rather than produce a program some kernel refuses to load.
#if defined(bpf_target_arm64)
// arm64 exposes struct user_pt_regs to user space, and struct pt_regs starts
// with it: x0-x30, sp, pc, pstate. Everything after it is kernel-internal.
#define CTX_STABLE_BYTES 272
_Static_assert(sizeof(struct user_pt_regs) == CTX_STABLE_BYTES,
               "struct user_pt_regs is UAPI and cannot change size");
_Static_assert(sizeof(struct pt_regs) >= CTX_STABLE_BYTES,
               "struct pt_regs must start with struct user_pt_regs");
#else
// x86-64's struct pt_regs is the whole ptrace register layout (r15 through
// ss); it has no kernel-internal tail.
#define CTX_STABLE_BYTES 168
_Static_assert(sizeof(struct pt_regs) == CTX_STABLE_BYTES,
               "x86-64 struct pt_regs is the ptrace register layout");
#endif

// Fails the build if a context field we read could fall outside the stable
// part of the layout.
#define ASSERT_CTX_FIELD_STABLE(field)                                      \
  _Static_assert(__builtin_offsetof(struct pt_regs, field) +                \
                         sizeof(((struct pt_regs*)0)->field) <=             \
                     CTX_STABLE_BYTES,                                      \
                 "reads of a probe's context must stay inside the stable "  \
                 "user-register area of struct pt_regs")

#define COPY_CTX_REG(dst, ctx, regno)                 \
  do {                                                \
    ASSERT_CTX_FIELD_STABLE(DWARF_REGISTER(regno));   \
    (dst)->regs[regno] = (ctx)->DWARF_REGISTER(regno); \
  } while (0)

// dwarf_regs_from_ctx snapshots the registers dyninst uses out of a probe's
// context. This is the only place dyninst reads the context.
static __always_inline void
dwarf_regs_from_ctx(dwarf_regs_t* dst, const struct pt_regs* ctx) {
  COPY_CTX_REG(dst, ctx, 0);
  COPY_CTX_REG(dst, ctx, 1);
  COPY_CTX_REG(dst, ctx, 2);
  COPY_CTX_REG(dst, ctx, 3);
  COPY_CTX_REG(dst, ctx, 4);
  COPY_CTX_REG(dst, ctx, 5);
  COPY_CTX_REG(dst, ctx, 6);
  COPY_CTX_REG(dst, ctx, 7);
  COPY_CTX_REG(dst, ctx, 8);
  COPY_CTX_REG(dst, ctx, 9);
  COPY_CTX_REG(dst, ctx, 10);
  COPY_CTX_REG(dst, ctx, 11);
  COPY_CTX_REG(dst, ctx, 12);
  COPY_CTX_REG(dst, ctx, 13);
  COPY_CTX_REG(dst, ctx, 14);
  COPY_CTX_REG(dst, ctx, 15);
#if defined(bpf_target_arm64)
  COPY_CTX_REG(dst, ctx, 16);
  COPY_CTX_REG(dst, ctx, 17);
  COPY_CTX_REG(dst, ctx, 18);
  COPY_CTX_REG(dst, ctx, 19);
  COPY_CTX_REG(dst, ctx, 20);
  COPY_CTX_REG(dst, ctx, 21);
  COPY_CTX_REG(dst, ctx, 22);
  COPY_CTX_REG(dst, ctx, 23);
  COPY_CTX_REG(dst, ctx, 24);
  COPY_CTX_REG(dst, ctx, 25);
  COPY_CTX_REG(dst, ctx, 26);
  COPY_CTX_REG(dst, ctx, 27);
  COPY_CTX_REG(dst, ctx, 28);
  COPY_CTX_REG(dst, ctx, 29);
  COPY_CTX_REG(dst, ctx, 30);
#endif
  ASSERT_CTX_FIELD_STABLE(DWARF_SP_REG);
  ASSERT_CTX_FIELD_STABLE(DWARF_PC_REG);
  dst->regs[DWARF_REGNO_SP] = ctx->DWARF_SP_REG;
  dst->regs[DWARF_REGNO_PC] = ctx->DWARF_PC_REG;
}

#undef COPY_CTX_REG

static __always_inline uint64_t dwarf_regs_sp(const dwarf_regs_t* regs) {
  return regs->regs[DWARF_REGNO_SP];
}

static __always_inline uint64_t dwarf_regs_fp(const dwarf_regs_t* regs) {
  return regs->regs[DWARF_REGNO_FP];
}

static __always_inline uint64_t dwarf_regs_pc(const dwarf_regs_t* regs) {
  return regs->regs[DWARF_REGNO_PC];
}

// Goroutine pointer (Go's g register).
static __always_inline uint64_t dwarf_regs_g(const dwarf_regs_t* regs) {
  return regs->regs[DWARF_REGNO_G];
}

// dwarf_regs_read reads a register by DWARF number. Returns false for
// register numbers this architecture doesn't have.
static __always_inline bool
dwarf_regs_read(const dwarf_regs_t* regs, uint32_t regno, uint64_t* out) {
  if (regno >= DWARF_REGNO_COUNT) {
    return false;
  }
  *out = regs->regs[regno];
  return true;
}

#endif // __REGS_H__
