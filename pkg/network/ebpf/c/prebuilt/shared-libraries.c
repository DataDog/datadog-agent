#include "kconfig.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_metadata.h"

#include <uapi/linux/ptrace.h>

#include "shared-libraries/types.h"
#include "shared-libraries/maps.h"
// all probes are shared among prebuilt and runtime, and can be found here
#include "shared-libraries/probes.h"

char _license[] SEC("license") = "GPL";
