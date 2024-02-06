#include "bpf_helpers.h"

SEC("ebpf_instrumentation/trampoline_handler")
int ebpf_instrumentation__trampoline_handler() {
    return 0;
}
