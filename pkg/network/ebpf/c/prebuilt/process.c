#include "kconfig.h"
#include <linux/types.h>
#include <linux/version.h>
#include <linux/bpf.h>

#include "defs.h"
#include "process.h"
#include "exec.h"
#include "container.h"

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
