#include "bpf_tracing.h"
#include "bpf_builtins.h"
#include "bpf_metadata.h"

#include "ktypes.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/ptrace.h>
#endif

#include "shared-libraries/types.h"
#include "shared-libraries/maps.h"
// all probes are shared among prebuilt and runtime, and can be found here
#include "shared-libraries/probes.h"

char _license[] SEC("license") = "GPL";
