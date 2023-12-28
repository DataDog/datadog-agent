#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include <uapi/linux/bpf.h>

char __license[] SEC("license") = "GPL";

SEC("xdp/ingress")
int logdebugtest(struct __sk_buff *skb) {
    log_debug("Hello, world!");
    log_debug("Goodbye, world!");
    return 42;
}
