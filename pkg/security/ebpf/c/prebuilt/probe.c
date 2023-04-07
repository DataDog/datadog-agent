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
#include <linux/module.h>
#include <linux/tty.h>
#include <linux/sched.h>
#include <linux/binfmts.h>
#include <linux/dcache.h>
#include <linux/mount.h>
#include <linux/fs.h>
#include <linux/magic.h>

#include <net/sock.h>
#include <net/netfilter/nf_conntrack.h>
#include <net/netfilter/nf_nat.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#include <uapi/linux/tcp.h>
#include <uapi/linux/utime.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
#pragma clang diagnostic ignored "-Wunused-function"

#include "bpf_tracing.h"
#include "hooks/all.h"

#pragma clang diagnostic pop

// unit tests
#ifdef __BALOUM__
#include "tests/tests.h"
#endif

__u32 _version SEC("version") = 0xFFFFFFFE;

char LICENSE[] SEC("license") = "GPL";
