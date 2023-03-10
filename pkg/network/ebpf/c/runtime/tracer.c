#include "kconfig.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"

#include <linux/tcp.h>
#include <linux/version.h>
#include <net/inet_sock.h>
#include <net/net_namespace.h>
#include <net/route.h>
#include <net/tcp_states.h>
#include <uapi/linux/if_ether.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/ptrace.h>
#include <uapi/linux/udp.h>

#include "tracer.h"
#include "tracer-events.h"
#include "tracer-maps.h"
#include "tracer-stats.h"
#include "tracer-telemetry.h"
#include "bpf_endian.h"
#include "ip.h"
#include "netns.h"
#include "sockfd.h"
#include "skb.h"
#include "port.h"
#include "tracer-bind.h"
#include "tracer-tcp.h"
#include "tracer-udp.h"

#include "protocols/classification/tracer-maps.h"
#include "protocols/classification/protocol-classification.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

#ifndef LINUX_VERSION_CODE
#error "kernel version not included?"
#endif

#include "sock.h"

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
