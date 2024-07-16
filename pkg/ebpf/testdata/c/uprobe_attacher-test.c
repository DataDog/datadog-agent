#include "kconfig.h"
#include "ktypes.h"
#include "bpf_metadata.h"
#include <uapi/linux/ptrace.h>
#include "bpf_tracing.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include <uapi/linux/bpf.h>

SEC("uprobe/SSL_connect")
int uprobe__SSL_connect(struct pt_regs *ctx) {
    return 0;
}

SEC("uprobe/main")
int uprobe__main(struct pt_regs *ctx) {
    return 0;
}
