#ifndef __GO_TLS_LOCATION_H
#define __GO_TLS_LOCATION_H

#include "bpf_helpers.h"

#define REG_SIZE 8

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
            // Verifier won't allow direct access to regs array if the index is not const
            switch (regnum) {
                case 0:
                    __builtin_memcpy(dest, &ctx->regs[0], sizeof(ctx->regs[0]));
                case 1:
                    __builtin_memcpy(dest, &ctx->regs[1], sizeof(ctx->regs[1]));
                case 2:
                    __builtin_memcpy(dest, &ctx->regs[2], sizeof(ctx->regs[2]));
                case 3:
                    __builtin_memcpy(dest, &ctx->regs[3], sizeof(ctx->regs[3]));
                case 4:
                    __builtin_memcpy(dest, &ctx->regs[4], sizeof(ctx->regs[4]));
                case 5:
                    __builtin_memcpy(dest, &ctx->regs[5], sizeof(ctx->regs[5]));
                case 6:
                    __builtin_memcpy(dest, &ctx->regs[6], sizeof(ctx->regs[6]));
                case 7:
                    __builtin_memcpy(dest, &ctx->regs[7], sizeof(ctx->regs[7]));
                case 8:
                    __builtin_memcpy(dest, &ctx->regs[8], sizeof(ctx->regs[8]));
                case 9:
                    __builtin_memcpy(dest, &ctx->regs[9], sizeof(ctx->regs[9]));
                case 10:
                    __builtin_memcpy(dest, &ctx->regs[10], sizeof(ctx->regs[10]));
                case 11:
                    __builtin_memcpy(dest, &ctx->regs[11], sizeof(ctx->regs[11]));
                case 12:
                    __builtin_memcpy(dest, &ctx->regs[12], sizeof(ctx->regs[12]));
                case 13:
                    __builtin_memcpy(dest, &ctx->regs[13], sizeof(ctx->regs[13]));
                case 14:
                    __builtin_memcpy(dest, &ctx->regs[14], sizeof(ctx->regs[14]));
                case 15:
                    __builtin_memcpy(dest, &ctx->regs[15], sizeof(ctx->regs[15]));
                default:
                    return 1;
            }
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
            // Verifier won't allow direct access to regs array if the index is not const
            switch (regnum) {
                case 0:
                    return &ctx->regs[0];
                case 1:
                    return &ctx->regs[1];
                case 2:
                    return &ctx->regs[2];
                case 3:
                    return &ctx->regs[3];
                case 4:
                    return &ctx->regs[4];
                case 5:
                    return &ctx->regs[5];
                case 6:
                    return &ctx->regs[6];
                case 7:
                    return &ctx->regs[7];
                case 8:
                    return &ctx->regs[8];
                case 9:
                    return &ctx->regs[9];
                case 10:
                    return &ctx->regs[10];
                case 11:
                    return &ctx->regs[11];
                case 12:
                    return &ctx->regs[12];
                case 13:
                    return &ctx->regs[13];
                case 14:
                    return &ctx->regs[14];
                case 15:
                    return &ctx->regs[15];
                default:
                    return NULL;
            }
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
    return bpf_probe_read_user(dest, size, (void*) address);
}

static __always_inline int read_location(struct pt_regs* ctx, location_t* loc, size_t size, void* dest) {
    if (!loc->exists) {
        return 0;
    }

    if (loc->in_register) {
        if (size != REG_SIZE) {
            return 1;
        }

        return read_register(ctx, loc->_register, dest);
    } else {
        return read_stack(ctx, loc->stack_offset, size, dest);
    }
}

#endif //__GO_TLS_LOCATION_H
