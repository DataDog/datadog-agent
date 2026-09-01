// This program is used to test the UprobeAttacher object, it defines simple probes that attach
// to userspace functions. The uretprobe exists so the uprobe_multi attach path's UretprobeMulti
// branch can be exercised, not just the UprobeMulti one.
#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_tracing.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"

SEC("uprobe/SSL_connect")
int uprobe__SSL_connect(struct pt_regs *ctx) {
    return 0;
}

SEC("uprobe/main")
int uprobe__main(struct pt_regs *ctx) {
    return 0;
}

SEC("uretprobe/SSL_connect")
int uretprobe__SSL_connect(struct pt_regs *ctx) {
    return 0;
}
