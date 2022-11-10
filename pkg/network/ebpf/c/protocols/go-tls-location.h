#ifndef __GO_TLS_LOCATION_H
#define __GO_TLS_LOCATION_H

#include "bpf_helpers.h"

#define REG_SIZE 8

// This function was adapted from https://github.com/go-delve/delve:
// - https://github.com/go-delve/delve/blob/cd9e6c02a6ca5f0d66c1f770ee10a0d8f4419333/pkg/proc/internal/ebpf/bpf/trace.bpf.c#L43
// which is licensed under MIT.
static __always_inline int read_register(struct pt_regs* ctx, int64_t regnum, void* dest) {
    #if defined(__x86_64__)
        // This volatile temporary variable is need when building with clang-14,
        // or the verifier will complain that we dereference a modified context
        // pointer.
        //
        // What happened in this case, is that the compiler tried to be smart by
        // incrementing the context pointer, before jumping to code that will
        // copy the value pointed to by the new pointer to `dest`. The generated
        // code looked like this:
        //
        //      r1 += 40           // Increment the ptr
        //      goto +3 <LBB0_9>   // goto __builtin_memcpy
        //
        // What the memcpy does is deference the resulting pointer to get the
        // CPU register value (that’s where the bug was), then put it in the
        // dest location:
        //
        //      r1 = *(u64 *)(r1 + 0)  // BUG: Get the register value.
        //                             // This is the "modified context pointer"
        //      *(u64 *)(r3 + 0) = r1  // Put it in dest
        //
        // By incrementing the pointer before dereferencing it, the verifier no
        // longer considering r1 to be a pointer to the context, but as a
        // pointer to some random memory address (even though it is in the
        // memory the range of the context struct).
        //
        // What we want the compiler to generate is something like this:
        //
        //      // Switch branch:
        //      r1 = *(u64 *)(r1 + 40) // read value to tmp var
        //      goto +30 <LBB0_39>     // goto *dest = tmp
        //
        //      // *dest = tmp
        //      *(u64 *)(r3 + 0) = r1
        //
        // This volatile `tmp` variable makes the compiler generate the code above.
        volatile u64 tmp = 0;
        switch (regnum) {
            case 0: // RAX
                tmp = ctx->ax;
                break;
            case 1: // RDX
                tmp = ctx->dx;
                break;
            case 2: // RCX
                tmp = ctx->cx;
                break;
            case 3: // RBX
                tmp = ctx->bx;
                break;
            case 4: // RSI
                tmp = ctx->si;
                break;
            case 5: // RDI
                tmp = ctx->di;
                break;
            case 6: // RBP
                tmp = ctx->bp;
                break;
            case 7: // RSP
                tmp = ctx->sp;
                break;
            case 8: // R8
                tmp = ctx->r8;
                break;
            case 9: // R9
                tmp = ctx->r9;
                break;
            case 10: // R10
                tmp = ctx->r10;
                break;
            case 11: // R11
                tmp = ctx->r11;
                break;
            case 12: // R12
                tmp = ctx->r12;
                break;
            case 13: // R13
                tmp = ctx->r13;
                break;
            case 14: // R14
                tmp = ctx->r14;
                break;
            case 15: // R15
                tmp = ctx->r15;
                break;
            default:
                      return 1;
        }
        *(u64*)dest = tmp;
        return 0;
    #elif defined(__aarch64__)
        // TODO Support ARM
        /*if (regnum >= 0 && regnum < sizeof(ctx->regs)) {
            // Verifier won't allow direct access to regs array if the index is not const
            switch (regnum) {
                case 0:
                    __builtin_memcpy(dest, &ctx->regs[0], sizeof(ctx->regs[0]));
                default:
                    return 1;
            }
            return 0;
        }*/
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
        // TODO Support ARM
        /*if (regnum >= 0 && regnum < sizeof(ctx->regs)) {
            // Verifier won't allow direct access to regs array if the index is not const
            switch (regnum) {
                case 0:
                    return &ctx->regs[0];
                default:
                    return NULL;
            }
        }*/
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
