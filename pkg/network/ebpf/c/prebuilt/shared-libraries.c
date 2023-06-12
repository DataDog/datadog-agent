#include "kconfig.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"

#include <uapi/linux/ptrace.h>

#include "shared-libraries/types.h"
#include "shared-libraries/maps.h"
#include "shared-libraries/probes.h"

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
