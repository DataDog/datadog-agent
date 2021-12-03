#include <linux/kconfig.h>

#include "bpf_helpers.h"
#include "tracer.h"
#include "conn-tuple.h"
#include "runtime-get-tls-base.h"
#include "go-tls-probes.h"

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
