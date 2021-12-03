#include <linux/kconfig.h>

#include "bpf_helpers.h"
#include "bpf_endian.h"
#include "tracer.h"
#include "ip.h"
#include "ipv6.h"
#include "sock.h"
#include "prebuilt-get-tls-base.h"
#include "go-tls-probes.h"

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
