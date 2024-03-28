#include "bpf_telemetry.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "compiler.h"

// ebpf_instrumentation__trampoline_handler is the target for the trampoline jump
// This program caches a pointer to the telemetry map on the stack at offset 512
SEC("ebpf_instrumentation/trampoline_handler")
int ebpf_instrumentation__trampoline_handler() {
    u64 key = 0;
    instrumentation_blob_t* tb = bpf_map_lookup_elem(&bpf_instrumentation_map, &key);
    if (tb == NULL) {
        asm ("r2 = 0");
        // we need to set this stack slot to avoid verifier error on access
        asm("*(u64 *)(r10 - 512) = r2");
        return 0;
    }

    // Cache telemetry blob on stack
    asm ("*(u64 *)(r10 - 512) = r0");
    return 0;
}
