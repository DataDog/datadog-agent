#ifndef HTTPS_MAPS_H_
#define HTTPS_MAPS_H_

#include "protocols/tls/http2.h"

#define HTTP2_TLS_MAX_SIZE 2048

BPF_PERCPU_ARRAY_MAP(http2_tls_info, __u32, http2_tls_info_t, 1)

BPF_PERCPU_ARRAY_MAP(http2_tls_iterations, __u32, http2_tail_call_state_t, 1)

#endif // HTTPS_MAPS_H_
