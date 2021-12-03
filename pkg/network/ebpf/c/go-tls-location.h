#ifndef __GO_TLS_LOCATION_H
#define __GO_TLS_LOCATION_H

#include "bpf_helpers.h"

// This function was adapted from https://github.com/go-delve/delve:
// - https://github.com/go-delve/delve/blob/cd9e6c02a6ca5f0d66c1f770ee10a0d8f4419333/pkg/proc/internal/ebpf/bpf/trace.bpf.c#L43
// which is licensed under MIT.
static __always_inline int read_register(struct pt_regs* ctx, int64_t regnum, void* dest) {
    #if defined(__x86_64__)
		switch (regnum) {
			case 0: // RAX
				__builtin_memcpy(dest, &ctx->ax, sizeof(ctx->ax));
				return 0;
			case 1: // RDX
				__builtin_memcpy(dest, &ctx->dx, sizeof(ctx->dx));
				return 0;
			case 2: // RCX
				__builtin_memcpy(dest, &ctx->cx, sizeof(ctx->cx));
				return 0;
			case 3: // RBX
				__builtin_memcpy(dest, &ctx->bx, sizeof(ctx->bx));
				return 0;
			case 4: // RSI
				__builtin_memcpy(dest, &ctx->si, sizeof(ctx->si));
				return 0;
			case 5: // RDI
				__builtin_memcpy(dest, &ctx->di, sizeof(ctx->di));
				return 0;
			case 6: // RBP
				__builtin_memcpy(dest, &ctx->bp, sizeof(ctx->bp));
				return 0;
			case 7: // RSP
				__builtin_memcpy(dest, &ctx->sp, sizeof(ctx->sp));
				return 0;
			case 8: // R8
				__builtin_memcpy(dest, &ctx->r8, sizeof(ctx->r8));
				return 0;
			case 9: // R9
				__builtin_memcpy(dest, &ctx->r9, sizeof(ctx->r9));
				return 0;
			case 10: // R10
				__builtin_memcpy(dest, &ctx->r10, sizeof(ctx->r10));
				return 0;
			case 11: // R11
				__builtin_memcpy(dest, &ctx->r11, sizeof(ctx->r11));
				return 0;
			case 12: // R12
				__builtin_memcpy(dest, &ctx->r12, sizeof(ctx->r12));
				return 0;
			case 13: // R13
				__builtin_memcpy(dest, &ctx->r13, sizeof(ctx->r13));
				return 0;
			case 14: // R14
				__builtin_memcpy(dest, &ctx->r14, sizeof(ctx->r14));
				return 0;
			case 15: // R15
				__builtin_memcpy(dest, &ctx->r15, sizeof(ctx->r15));
				return 0;
			default:
				return 1;
		}
    #elif defined(__aarch64__)
		if (regnum >= 0 && regnum < sizeof(ctx->regs)) {
			__builtin_memcpy(dest, &ctx->regs[regnum], sizeof(ctx->regs[regnum]));
			return 0;
		}
		return 1;
    #else
		#error "Unsupported platform"
    #endif
}

// This function was adapted from https://github.com/go-delve/delve:
// - https://github.com/go-delve/delve/blob/cd9e6c02a6ca5f0d66c1f770ee10a0d8f4419333/pkg/proc/internal/ebpf/bpf/trace.bpf.c#L43
// which is licensed under MIT.
static __always_inline void* read_register_indirect(struct pt_regs* ctx, int64_t regnum) {
    #if defined(__x86_64__)
		switch (regnum) {
			case 0: // RAX
				return &ctx->ax;
			case 1: // RDX
				return &ctx->dx;
			case 2: // RCX
				return &ctx->cx;
			case 3: // RBX
				return &ctx->bx;
			case 4: // RSI
				return &ctx->si;
			case 5: // RDI
				return &ctx->di;
			case 6: // RBP
				return &ctx->bp;
			case 7: // RSP
				return &ctx->sp;
			case 8: // R8
				return &ctx->r8;
			case 9: // R9
				return &ctx->r9;
			case 10: // R10
				return &ctx->r10;
			case 11: // R11
				return &ctx->r11;
			case 12: // R12
				return &ctx->r12;
			case 13: // R13
				return &ctx->r13;
			case 14: // R14
				return &ctx->r14;
			case 15: // R15
				return &ctx->r15;
			default:
				return NULL;
		}
    #elif defined(__aarch64__)
		if (regnum >= 0 && regnum < sizeof(ctx->regs)) {
			return &ctx->regs[regnum];
		}
		return NULL;
    #else
		#error "Unsupported platform"
    #endif
}

static __always_inline int read_stack(struct pt_regs* ctx, int64_t stack_offset, size_t size, void* dest) {
	// `ctx->sp` is correct for both x86_64 and ARM64
	uintptr_t stack_pointer = (uintptr_t) ctx->sp;
	uintptr_t address = stack_pointer + stack_offset;
	return bpf_probe_read(dest, size, (void*) address);
}

static __always_inline int read_location(struct pt_regs* ctx, location_t* loc, size_t size, void* dest) {
	if (!loc->exists) {
		return 0;
	}

	if (loc->in_register) {
        if (size != 8) {
            return 1;
        }

		return read_register(ctx, loc->_register, dest);
	} else {
		return read_stack(ctx, loc->stack_offset, size, dest);
	}
}

#endif //__GO_TLS_LOCATION_H
