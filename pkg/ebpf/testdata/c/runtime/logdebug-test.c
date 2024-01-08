#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include <uapi/linux/bpf.h>

char __license[] SEC("license") = "GPL";

SEC("kprobe/do_vfs_ioctl")
int logdebugtest(struct pt_regs *ctx) {
    log_debug("hi"); // small word, should get a single MovImm instruction
    log_debug("123456"); // Small word, single movImm instruction on 64-bit boundary (add 2 bytes for newline and null character)
    log_debug("1234567"); // null character has to go on next 64b word
    log_debug("12345678"); // newline and null character have to go on next word
    log_debug("Goodbye, world!"); // Medium sized, should get several loads. Also newline here falls on a 64-bit boundary
    log_debug("even more words a lot of words here should be several instructions");
    log_debug("with args: 2+2=%d", 4);
    int a = 1;
    int b = 2;
    log_debug("with more args and vars: %d+%d=%d", a, b, a + b);
    log_debug("bye");

    return 0;
}
