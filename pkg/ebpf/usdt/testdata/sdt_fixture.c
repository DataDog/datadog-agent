// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Source of the sdt_fixture_*.so test fixtures of pkg/ebpf/usdt: a
// freestanding shared object whose .note.stapsdt notes cover the cases
// handled by ReadProbes. It is never executed. See the Makefile to rebuild.
//
// SDT_NOTE emits a probe note like STAP_PROBE of SystemTap's <sys/sdt.h>, but
// with explicit expressions for the three addresses of the note (site,
// .stapsdt.base, semaphore) and a literal argument string, so that the
// fixture content does not depend on the compiler's register allocation.

#define SDT_NOTE_TYPE(type, loc, base, sem, provider, name, args) \
    __asm__ __volatile__(                                         \
        "990: nop\n"                                              \
        ".pushsection .note.stapsdt, \"\", \"note\"\n"            \
        ".balign 4\n"                                             \
        ".4byte 992f-991f, 994f-993f, " #type "\n"                \
        "991: .asciz \"stapsdt\"\n"                               \
        "992: .balign 4\n"                                        \
        "993: .8byte " loc "\n"                                   \
        ".8byte " base "\n"                                       \
        ".8byte " sem "\n"                                        \
        ".asciz \"" provider "\"\n"                               \
        ".asciz \"" name "\"\n"                                   \
        ".asciz \"" args "\"\n"                                   \
        "994: .balign 4\n"                                        \
        ".popsection\n"                                           \
        ".ifndef _.stapsdt.base\n"                                \
        ".pushsection .stapsdt.base, \"aG\", \"progbits\", .stapsdt.base, comdat\n" \
        ".weak _.stapsdt.base\n"                                  \
        ".hidden _.stapsdt.base\n"                                \
        "_.stapsdt.base: .space 1\n"                              \
        ".size _.stapsdt.base, 1\n"                               \
        ".popsection\n"                                           \
        ".endif\n")

#define SDT_NOTE(loc, base, sem, provider, name, args) \
    SDT_NOTE_TYPE(3, loc, base, sem, provider, name, args)

// A regular probe site, as emitted by <sys/sdt.h>.
#define SDT_PROBE(name, args) SDT_NOTE("990b", "_.stapsdt.base", "0", "test", name, args)

// Basic asm: '%' is not an escape character.
#if defined(__x86_64__)
#define ARGS_MIXED "-4@-20(%rbp) 8@%rax 1@$1 -2@%r10w"
#define ARGS_REG "8@%rdi"
#define ARGS_SIB "8@(%rax,%rbx,8)"
#elif defined(__aarch64__)
#define ARGS_MIXED "-4@[sp, 60] 8@x1 1@1 -2@w10"
#define ARGS_REG "8@x0"
#define ARGS_SIB "8@[x0, x1]"
#else
#error "unsupported architecture"
#endif

// Semaphore of the test:semaphore probe, in the .probes section as with
// <sys/sdt.h>.
__attribute__((section(".probes"), used, visibility("hidden"))) volatile unsigned short test_semaphore_semaphore;

// Data object, outside of any executable segment.
__attribute__((used, visibility("hidden"))) int fixture_data = 1;

// Function whose first instruction is not a nop.
void fixture_ret(void) {
}

void fixture(void) {
    // Regular sites: one with arguments, and two sites of the same probe
    // without arguments.
    SDT_PROBE("args", ARGS_MIXED);
    SDT_PROBE("noargs", "");
    SDT_PROBE("noargs", "");

    // Site of a prelinked file: the note records the addresses of the site
    // and of .stapsdt.base 0x1000 bytes lower than they are in the file.
    SDT_NOTE("990b - 0x1000", "_.stapsdt.base - 0x1000", "0", "test", "prelinked", ARGS_REG);

    // Site whose note has no base address: no prelink adjustment.
    SDT_NOTE("990b", "0", "0", "test", "nobase", "");

    // Site with a semaphore.
    SDT_NOTE("990b", "_.stapsdt.base", "test_semaphore_semaphore", "test", "semaphore", "");

    // Site with unsupported arguments.
    SDT_PROBE("sib", ARGS_SIB);

    // Sites that must not be attached: outside an executable segment, or not
    // on a nop instruction.
    SDT_NOTE("fixture_data", "_.stapsdt.base", "0", "test", "data", "");
    SDT_NOTE("fixture_ret", "_.stapsdt.base", "0", "test", "notnop", "");

    // Notes ignored by ReadProbes: wrong note type.
    SDT_NOTE_TYPE(1, "990b", "_.stapsdt.base", "0", "test", "v1", "");
}
