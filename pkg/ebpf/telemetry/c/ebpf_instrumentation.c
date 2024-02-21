#include "bpf_telemetry.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "compiler.h"

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

    u64 program_index = 0;
    LOAD_CONSTANT("telemetry_program_id_key", program_index);

    // Cache telemetry blob on stack
    asm ("*(u64 *)(r10 - 512) = r0");
    return 0;
}
