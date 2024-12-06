#ifndef TELEMETRY_TYPES_H
#define TELEMETRY_TYPES_H

//#include <uapi/asm-generic/errno-base.h>

// We use a power of 2 array size so the upper bound of a map
// access can be easily constrained with an 'and' operation
#define T_MAX_ERRNO 64

typedef struct {
    unsigned long err_count[T_MAX_ERRNO];
} map_err_telemetry_t;

#define bpf_probe_read_indx         0
#define bpf_probe_read_user_indx    1
#define bpf_probe_read_kernel_indx  2
#define bpf_skb_load_bytes_indx     3
#define bpf_perf_event_output_indx  4
#define bpf_ringbuf_output_indx     5
#define MAX_TELEMETRY_INDX          6
typedef struct {
    unsigned long err_count[MAX_TELEMETRY_INDX * T_MAX_ERRNO];
} helper_err_telemetry_t;

#endif
