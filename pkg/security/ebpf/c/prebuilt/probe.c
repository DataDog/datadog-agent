#include "bpf_metadata.h"

#include "kconfig.h"
#include <linux/types.h>
#include <linux/version.h>

#include <net/sock.h>
#include <net/netfilter/nf_conntrack.h>
#include <net/netfilter/nf_nat.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#include <uapi/linux/tcp.h>
#include <uapi/linux/utime.h>
#include <uapi/linux/ptrace.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

#include "bpf_tracing.h"
#include "hooks/all.h"

#pragma clang diagnostic pop

// unit tests
#ifdef __BALOUM__
#include "tests/tests.h"
#endif

char LICENSE[] SEC("license") = "GPL";
