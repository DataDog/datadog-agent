#ifndef __USDT_TYPES_H
#define __USDT_TYPES_H

// Types shared between the eBPF USDT helpers of usdt.h and the Go package
// pkg/ebpf/usdt, which generates its Go mirror from this file with cgo -godefs.
// Keep this header free of any eBPF-only dependency.

#include "ktypes.h"

// Maximum number of arguments of a USDT probe, as in SystemTap's sys/sdt.h.
#define USDT_MAX_ARG_CNT 12

// Number of entries of the usdt_specs map, i.e. of distinct specs that can be
// in use at the same time.
#define USDT_MAX_SPEC_CNT 256

// Location of a USDT argument.
enum usdt_arg_type {
    // The argument is a constant, stored in val_off ("1@$1" on x86-64, "1@1" on arm64).
    USDT_ARG_CONST,
    // The argument is the value of the register at offset reg_off of struct pt_regs
    // ("8@%rax" on x86-64, "8@x1" on arm64).
    USDT_ARG_REG,
    // The argument is in user memory, at the address held by the register at
    // offset reg_off of struct pt_regs plus val_off ("-4@-20(%rbp)" on x86-64,
    // "8@[sp, 112]" on arm64).
    USDT_ARG_REG_DEREF,
};

struct usdt_arg_spec {
    // Value of a USDT_ARG_CONST argument, or offset added to the register of a
    // USDT_ARG_REG_DEREF argument.
    __u64 val_off;
    // enum usdt_arg_type
    __u32 arg_type;
    // Offset of the register in struct pt_regs.
    __s16 reg_off;
    // Whether the argument is sign-extended to 64 bits.
    __u8 arg_signed;
    // 64 - 8 * size of the argument in bytes: 0, 32, 48 or 56.
    __u8 arg_bitshift;
};

// Argument specification of a USDT probe site.
struct usdt_spec {
    struct usdt_arg_spec args[USDT_MAX_ARG_CNT];
    __u16 arg_cnt;
    __u8 pad[6];
};

#endif
