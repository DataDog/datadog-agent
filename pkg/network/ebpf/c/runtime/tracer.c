#include "kconfig.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"

#include "tracer-bind.h"
#include "tracer-tcp.h"
#include "tracer-udp.h"

#include "protocols/classification/tracer-maps.h"
#include "protocols/classification/protocol-classification.h"

#ifndef LINUX_VERSION_CODE
#error "kernel version not included?"
#endif

SEC("socket/classifier_entry")
int socket__classifier_entry(struct __sk_buff *skb) {
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
    protocol_classifier_entrypoint(skb);
    #endif
    return 0;
}

SEC("socket/classifier_queues")
int socket__classifier_queues(struct __sk_buff *skb) {
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
    protocol_classifier_entrypoint_queues(skb);
    #endif
    return 0;
}

SEC("socket/classifier_dbs")
int socket__classifier_dbs(struct __sk_buff *skb) {
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
    protocol_classifier_entrypoint_dbs(skb);
    #endif
    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
