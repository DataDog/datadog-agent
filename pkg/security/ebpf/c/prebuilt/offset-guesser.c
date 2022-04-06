#include <linux/compiler.h>

#include "kconfig.h"
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>
#include <linux/bpf.h>
#include <linux/filter.h>
#include <uapi/asm-generic/mman-common.h>
#include <linux/pipe_fs_i.h>
#include <linux/nsproxy.h>

#include <net/sock.h>
#include <net/netfilter/nf_conntrack.h>
#include <net/netfilter/nf_nat.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#include <uapi/linux/tcp.h>

#include "defs.h"
#include "offset.h"

__u32 _version SEC("version") = 0xFFFFFFFE;

char LICENSE[] SEC("license") = "GPL";
