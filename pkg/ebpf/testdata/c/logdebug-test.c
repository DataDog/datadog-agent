#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include <uapi/linux/bpf.h>

char __license[] SEC("license") = "GPL";

int nested_func(int a, int b) {
    // a = b = 50
    a += 20; // a = 70
    b += bpf_get_smp_processor_id() - bpf_get_smp_processor_id(); // Compiler doesn't know this is always zero

    if (a > b) {
        return a; // 70
    }

    return b;
}

// A function that simulates instructions being added in the middle of the log_debug call
int somefunc(unsigned int number) {
    // Call another function
    u32 pid = bpf_get_smp_processor_id();

    // The compiler is damn smart and if we use pid in such a way that
    // the result is always constant (which is useful to have consistent, reliable tests)
    // then it will optimize everything away. So we use a reasonable assumption: no
    // system we run this on is going to have a million CPUs. For us, that means
    // that we can be sure that pid = 0, so we can create many operations that
    // will get compiled to assembly.

    if (pid < 1000000) {
        pid = 0;
    }

    number += pid; // 80
    number /= 2; // 40
    number += 10; // 50
    number += nested_func(pid, pid); // 70

    return pid + number;
}

SEC("kprobe/do_vfs_ioctl")
int logdebugtest(struct pt_regs *ctx) {
    log_debug("hi"); // small word, should get a single MovImm instruction
    log_debug("123456"); // Small word, single movImm instruction on 64-bit boundary (add 2 bytes for newline and null character)
    log_debug("1234567"); // null character has to go on next 64b word
    log_debug("12345678"); // newline and null character have to go on next word
    log_debug("Goodbye, world!"); // Medium sized, should get several loads. Also newline here falls on a 64-bit boundary
    log_debug("even more words a lot of words here should be several instructions");

    log_debug("12"); // Check with a small word...
    log_debug("21"); // and another of the same length to see what does the compiler with that

    log_debug("with args: 2+2=%d", 4);
    int a = 1;
    int b = 2;
    log_debug("with more args and vars: %d+%d=%d", a, b, a + b); // Funnily enough, the last dword for the string is the same as in the previous log_debug call, so the compiler reuses the same register
    log_debug("with a function call in the argument: %d and more words so that I force the compiler to not reuse register", somefunc(80));
    log_debug("bye");

    return 0;
}
