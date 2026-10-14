#ifndef __USDT_H
#define __USDT_H

// Helpers to read the arguments of USDT (SystemTap SDT) probes from uprobe
// programs, for CO-RE builds on x86-64 and arm64.
//
// The design follows libbpf's usdt.bpf.h (Copyright (c) 2022 Meta Platforms,
// Inc., LGPL-2.1 OR BSD-2-Clause): user space parses the argument
// specification of every probe site (pkg/ebpf/usdt), stores it in the
// usdt_specs array map, and the eBPF program decodes the arguments from
// struct pt_regs and user memory. Unlike libbpf, the spec id is not looked up
// from a BPF cookie or from the instruction pointer: it is a constant patched
// into the program with the "usdt_spec_id" constant editor, so a program
// instance (or clone) serves every probe site that shares the same spec.
//
// Spec id 0 is never allocated by pkg/ebpf/usdt.SpecRegistry: a program
// whose "usdt_spec_id" constant is not edited reads the zeroed entry 0, so
// usdt_arg() fails with -ENOENT instead of decoding the arguments of another
// site.
//
// bpf_probe_read_kernel and bpf_probe_read_user are used unconditionally:
// cilium/ebpf rewrites them to bpf_probe_read when loading on kernels that do
// not have them (< 5.5).

#include "ktypes.h"
#include "bpf_helpers.h"
#include "compiler.h"
#include "map-defs.h"
#include "usdt_types.h"

#if __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
#error "usdt.h only supports little-endian targets"
#endif

#ifndef ENOENT
#define ENOENT 2
#endif
#ifndef ESRCH
#define ESRCH 3
#endif
#ifndef EINVAL
#define EINVAL 22
#endif

BPF_ARRAY_MAP(usdt_specs, struct usdt_spec, USDT_MAX_SPEC_CNT)

// usdt_get_spec returns the spec of the probe site the program is attached
// to, or NULL.
static __always_inline struct usdt_spec *usdt_get_spec(void) {
    __u64 spec_id = 0;
    LOAD_CONSTANT("usdt_spec_id", spec_id);
    __u32 key = spec_id;
    return bpf_map_lookup_elem(&usdt_specs, &key);
}

// usdt_arg reads the 0-based argument arg_num of the probe site the program
// is attached to, sign- or zero-extended to 64 bits, into *res.
// It returns 0 on success, or a negative error, in which case *res is 0.
static __always_inline int usdt_arg(struct pt_regs *ctx, __u64 arg_num, long *res) {
    struct usdt_spec *spec;
    struct usdt_arg_spec *arg;
    unsigned long val = 0;
    unsigned long addr;
    int size;
    int err;

    *res = 0;

    spec = usdt_get_spec();
    if (!spec) {
        return -ESRCH;
    }

    if (arg_num >= USDT_MAX_ARG_CNT) {
        return -ENOENT;
    }
    // keep the bound check of arg_num on the register used to index args
    barrier_var(arg_num);
    if (arg_num >= spec->arg_cnt) {
        return -ENOENT;
    }
    arg = &spec->args[arg_num];

    // the bit shift encodes the size of the argument; validating it keeps the
    // shifts below in [0, 64) and the size of the user memory read in [1, 8]
    switch (arg->arg_bitshift) {
    case 0:
        size = 8;
        break;
    case 32:
        size = 4;
        break;
    case 48:
        size = 2;
        break;
    case 56:
        size = 1;
        break;
    default:
        return -EINVAL;
    }

    switch (arg->arg_type) {
    case USDT_ARG_CONST:
        val = arg->val_off;
        break;
    case USDT_ARG_REG:
    case USDT_ARG_REG_DEREF:
        if (arg->reg_off < 0 || arg->reg_off > (int)(sizeof(struct pt_regs) - sizeof(val))) {
            return -EINVAL;
        }
        err = bpf_probe_read_kernel(&val, sizeof(val), (char *)ctx + arg->reg_off);
        if (err) {
            return err;
        }
        if (arg->arg_type == USDT_ARG_REG) {
            break;
        }
        // read exactly the size of the argument, into the low-order bytes of
        // val (little-endian), so that a read never crosses past its end
        addr = val + arg->val_off;
        val = 0;
        err = bpf_probe_read_user(&val, size, (void *)addr);
        if (err) {
            return err;
        }
        break;
    default:
        return -EINVAL;
    }

    // keep the low-order 8*size bits, sign-extended if the argument is signed
    val <<= arg->arg_bitshift;
    if (arg->arg_signed) {
        val = ((long)val) >> arg->arg_bitshift;
    } else {
        val >>= arg->arg_bitshift;
    }
    *res = val;
    return 0;
}

#endif
