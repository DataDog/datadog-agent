#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include <uapi/linux/bpf.h>

char __license[] SEC("license") = "GPL";

SEC("kprobe/do_vfs_ioctl")
int logdebugtest(struct pt_regs *ctx) {
    log_debug("Hello, world!");
    log_debug("Goodbye, world!");

    return 0;
}
