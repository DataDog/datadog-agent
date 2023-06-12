#include "bpf_tracing.h"
#include "bpf_builtins.h"

#include "ktypes.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/ptrace.h>
#endif

#include "shared-libraries/types.h"
#include "shared-libraries/maps.h"
#include "shared-libraries/probes.h"

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
