// This program is used to test if the uretprobe syscall bug described in the commit below is present
// https://www.mail-archive.com/linux-trace-kernel@vger.kernel.org/msg04970.html
#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_tracing.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"

SEC("uretprobe/segfault")
int uretprobe__segfault(struct pt_regs *ctx) {
    return 0;
}
