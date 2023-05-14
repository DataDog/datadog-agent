#include <linux/compiler.h>

#include "kconfig.h"
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>
#include <linux/bpf.h>

#include "bpf_tracing.h"

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

#include "map-defs.h"

BPF_ARRAY_MAP(guessed_offsets, u32, 2)

#define PID_OFFSET_INDEX 0
#define MIN_PID_OFFSET 32
#define MAX_PID_OFFSET 256
#define PID_STRUCT_OFFSET_INDEX 1
#define MIN_PID_STRUCT_OFFSET 1024
#define MAX_PID_STRUCT_OFFSET 3192

#include "hooks/offset_guesser.h"

#pragma clang diagnostic pop

__u32 _version SEC("version") = 0xFFFFFFFE;

char LICENSE[] SEC("license") = "GPL";
