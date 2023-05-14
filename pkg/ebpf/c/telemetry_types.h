#ifndef TELEMETRY_TYPES_H
#define TELEMETRY_TYPES_H

//#include <uapi/asm-generic/errno-base.h>

// We use a power of 2 array size so the upper bound of a map
// access can be easily constrained with an 'and' operation
#define T_MAX_ERRNO 64

typedef struct {
    unsigned long err_count[T_MAX_ERRNO];
} map_err_telemetry_t;

#define read_indx 0
#define read_user_indx 1
#define read_kernel_indx 2
#define skb_load_bytes 3
#define perf_event_output 4
#define MAX_TELEMETRY_INDX 5
typedef struct {
    unsigned long err_count[MAX_TELEMETRY_INDX * T_MAX_ERRNO];
} helper_err_telemetry_t;

#endif
