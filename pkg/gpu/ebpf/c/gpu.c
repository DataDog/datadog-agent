#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"

char __license[] SEC("license") = "GPL";

SEC("uprobe/cudaLaunchKernel")
int uprobe_cudaLaunchKernel(struct pt_regs *ctx) {
    bpf_printk("hi from cudaLaunchKernel\n");

    return 0;
}
