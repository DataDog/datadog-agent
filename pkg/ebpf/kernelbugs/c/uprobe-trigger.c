// This program is used to test if the uretprobe syscall bug described in the commit below is present
// https://www.mail-archive.com/linux-trace-kernel@vger.kernel.org/msg04970.html
#include "bpf_helpers.h"

SEC("uretprobe/segfault")
int uretprobe__segfault(struct pt_regs *ctx) {
    return 0;
}
