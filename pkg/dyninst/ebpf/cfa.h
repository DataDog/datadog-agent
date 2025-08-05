#ifndef __CFA_H__
#define __CFA_H__

#include "bpf_helpers.h"
#include "bpf_tracing.h"

static inline uint64_t calculate_cfa(struct pt_regs* regs, bool frameless) {
  // Stack layout is slightly different in Go between arm64 and x86_64.
  // Established based on following documentation and machine code reads:
  // https://tip.golang.org/src/cmd/compile/abi-internal#architecture-specifics

  // Code examples:

  // amd64, framefull function with inlined function

  // Target variable in executeInlined
  // 0x000450c3:     DW_TAG_variable
  //                   DW_AT_name    ("a")
  //                   DW_AT_decl_line       (76)
  //                   DW_AT_type    (0x0000000000242414 "int[5]")
  //                   DW_AT_location        (0x00087bf9: 
  //                      [0x0000000000cd4cbb, 0x0000000000cd4d55): DW_OP_fbreg -56)

  // Target variable in testInlinedSumArray
  // 0x000450e9:       DW_TAG_formal_parameter
  //                     DW_AT_abstract_origin       (0x000000000003e970 "a")
  //                     DW_AT_location      (0x00087c2d: 
  //                        [0x0000000000cd4ce4, 0x0000000000cd4d55): DW_OP_fbreg -96)

  // 0000000000cd4ca0 <main.executeInlined>:
  // ; func executeInlined() {
  //   cd4ca0: 49 3b 66 10                   cmpq    16(%r14), %rsp
  //   cd4ca4: 0f 86 a1 00 00 00             jbe     0xcd4d4b <main.executeInlined+0xab>
  //   cd4caa: 55                            pushq   %rbp
  //   cd4cab: 48 89 e5                      movq    %rsp, %rbp
  // Injection point for executeInlined
  // %rsp == %rbp
  // array offset == %rsp-104+64 == %rsp-40 == (%rbp+16)-56
  //   cd4cae: 48 83 ec 68                   subq    $104, %rsp
  // ;       a := [5]int{1, 2, 3, 4, 5}
  //   cd4cb2: 48 c7 44 24 40 01 00 00 00    movq    $1, 64(%rsp)
  //   cd4cbb: 48 c7 44 24 48 02 00 00 00    movq    $2, 72(%rsp)
  //   cd4cc4: 48 c7 44 24 50 03 00 00 00    movq    $3, 80(%rsp)
  //   cd4ccd: 48 c7 44 24 58 04 00 00 00    movq    $4, 88(%rsp)
  //   cd4cd6: 48 c7 44 24 60 05 00 00 00    movq    $5, 96(%rsp)
  // ;       y := testInlinedSumArray(a)
  //   cd4cdf: 48 8b 4c 24 40                movq    64(%rsp), %rcx
  //   cd4ce4: 48 89 4c 24 18                movq    %rcx, 24(%rsp)
  //   cd4ce9: 0f 10 44 24 48                movups  72(%rsp), %xmm0
  //   cd4cee: 0f 11 44 24 20                movups  %xmm0, 32(%rsp)
  //   cd4cf3: 0f 10 44 24 58                movups  88(%rsp), %xmm0
  //   cd4cf8: 0f 11 44 24 30                movups  %xmm0, 48(%rsp)
  // ;       return a[0] + a[1] + a[2] + a[3] + a[4]
  // Injection point for testInlinedSumArray
  // %rsp == %rbp-104
  // array offset == %rsp + 24 == %rbp-104+24 == %rbp-80 == (%rbp+16)-96
  //   cd4cfd: 48 8b 5c 24 18                movq    24(%rsp), %rbx
  //   cd4d02: 48 03 5c 24 20                addq    32(%rsp), %rbx
  //   cd4d07: 48 03 5c 24 28                addq    40(%rsp), %rbx
  //   cd4d0c: 48 03 5c 24 30                addq    48(%rsp), %rbx
  //   cd4d11: 48 03 5c 24 38                addq    56(%rsp), %rbx
  //   cd4d16: 48 89 5c 24 10                movq    %rbx, 16(%rsp)

  // arm64, function inlined into framefull

  // Target variable in executeInlined
  // 0x0004336f:     DW_TAG_variable
  //                   DW_AT_name    ("a")
  //                   DW_AT_decl_line       (76)
  //                   DW_AT_type    (0x00000000002425fe "int[5]")
  //                   DW_AT_location        (0x000862b8: 
  //                      [0x000000000084a144, 0x000000000084a1e0): DW_OP_fbreg -48)

  // Target variable in testInlinedSumArray
  // 0x00043395:       DW_TAG_formal_parameter
  //                     DW_AT_abstract_origin       (0x000000000003ccfe "a")
  //                     DW_AT_location      (0x000862ec: 
  //                        [0x000000000084a164, 0x000000000084a1e0): DW_OP_fbreg -88)

  // 000000000084a120 <main.executeInlined>:
  // sp == x29+8
  // ; func executeInlined() {
  //   84a120: 90 0b 40 f9   ldr     x16, [x28, #16]
  //   84a124: ff 63 30 eb   cmp     sp, x16
  //   84a128: 29 05 00 54   b.ls    0x84a1cc <main.executeInlined+0xac>
  // Injection point for executeInlined
  // array offset == sp+80 == (x29+8)-128+80 == (x29+8)-48
  //   84a12c: fe 0f 18 f8   str     x30, [sp, #-128]!
  //   84a130: fd 83 1f f8   stur    x29, [sp, #-8]
  //   84a134: fd 23 00 d1   sub     x29, sp, #8
  // ;       a := [5]int{1, 2, 3, 4, 5}
  //   84a138: 3b 20 00 90   adrp    x27, 0xc4e000 <main.executeMapFuncs+0x798>
  //   84a13c: 7b 43 23 91   add     x27, x27, #2256
  //   84a140: 62 0f 40 a9   ldp     x2, x3, [x27]
  //   84a144: 3b 20 00 90   adrp    x27, 0xc4e000 <main.executeMapFuncs+0x7a4>
  //   84a148: 7b 83 23 91   add     x27, x27, #2272
  //   84a14c: 64 17 40 a9   ldp     x4, x5, [x27]
  //   84a150: e2 0f 05 a9   stp     x2, x3, [sp, #80]
  //   84a154: e4 17 06 a9   stp     x4, x5, [sp, #96]
  //   84a158: a2 00 80 d2   mov     x2, #5
  //   84a15c: e2 3b 00 f9   str     x2, [sp, #112]
  // ;       y := testInlinedSumArray(a)
  //   84a160: e2 0f 45 a9   ldp     x2, x3, [sp, #80]
  //   84a164: e4 17 46 a9   ldp     x4, x5, [sp, #96]
  //   84a168: e2 8f 02 a9   stp     x2, x3, [sp, #40]
  //   84a16c: e4 97 03 a9   stp     x4, x5, [sp, #56]
  //   84a170: e2 3b 40 f9   ldr     x2, [sp, #112]
  //   84a174: e2 27 00 f9   str     x2, [sp, #72]
  // ;       return a[0] + a[1] + a[2] + a[3] + a[4]
  // Injection point for testInlinedSumArray
  // sp == [sp-8]+8-128 == [sp-8]-120
  // sp+40 == [sp-8]-120+40 = [sp-8]-80 = ([sp-8]+8)-88
  //   84a178: e2 17 40 f9   ldr     x2, [sp, #40]
  //   84a17c: e3 1b 40 f9   ldr     x3, [sp, #48]
  //   84a180: 62 00 02 8b   add     x2, x3, x2
  //   84a184: e3 1f 40 f9   ldr     x3, [sp, #56]
  //   84a188: 62 00 02 8b   add     x2, x3, x2
  //   84a18c: e3 23 40 f9   ldr     x3, [sp, #64]
  //   84a190: 62 00 02 8b   add     x2, x3, x2
  //   84a194: e3 27 40 f9   ldr     x3, [sp, #72]
  //   84a198: 61 00 02 8b   add     x1, x3, x2
  //   84a19c: e1 13 00 f9   str     x1, [sp, #32]

  // amd64, frameless function

  // 0x00043f62:     DW_TAG_formal_parameter
  //                   DW_AT_name    ("x")
  //                   DW_AT_variable_parameter      (0x00)
  //                   DW_AT_decl_line       (18)
  //                   DW_AT_type    (0x0000000000122012 "int32[2]")
  //                   DW_AT_location        (DW_OP_call_frame_cfa)

  // Call site
  // ;        ([2]byte{1, 1})
  //   cd3b3f: 66 c7 04 24 01 01             movw    $257, (%rsp)            # imm = 0x101
  //   cd3b45: e8 36 fd ff ff                callq   0xcd3880 <main.testByteArray>

  // ; func testByteArray(x [2]byte) {}
  // Injection point
  // array offset == %rsp+8 (was ==rsp before a call which pushed 8 bytes on stack)
  //   cd3880: c3                            retq

  // arm64, frameless function

  // 0x000421c1:     DW_TAG_formal_parameter
  //                   DW_AT_name    ("x")
  //                   DW_AT_variable_parameter      (0x00)
  //                   DW_AT_decl_line       (14)
  //                   DW_AT_type    (0x000000000011a5e1 "uint8[2]")
  //                   DW_AT_location        (DW_OP_fbreg +8)

  // Call site
  // sp == x29+8
  //   849288: e0 13 00 79   strh    w0, [sp, #8]
  //   84928c: 9d ff ff 97   bl      0x849100 <main.testByteArray>

  // ; func testByteArray(x [2]byte) {}
  // Injection point
  // array offset == sp == x29+8
  //   849100: c0 03 5f d6   ret

  #if defined(bpf_target_arm64)
  if (frameless) {
    return regs->DWARF_BP_REG + 8;
  } else {
    uint64_t bp;
    if (bpf_probe_read_user(&bp, sizeof(bp), (void*)(regs->DWARF_SP_REG-8)) != 0) {
      return 0;
    }
    return bp + 8;
  }
  #elif defined(bpf_target_x86)
  if (frameless) {
    return regs->DWARF_SP_REG + 8;
  } else {
    return regs->DWARF_BP_REG + 16;
  }
  #else
      #error "Unsupported architecture"
  #endif
}

#endif // __CFA_H__
