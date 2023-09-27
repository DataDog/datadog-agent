#ifndef HTTPS_MAPS_H_
#define HTTPS_MAPS_H_

#include "protocols/http2/decoding-defs.h"

#define HTTP2_TLS_MAX_SIZE 2048

BPF_HASH_MAP(http2_tls_states, http2_tls_state_key_t, http2_tls_state_t, 10)
BPF_PERCPU_ARRAY_MAP(http2_tls_iterations, __u32, http2_tail_call_state_t, 1)

#endif // HTTPS_MAPS_H_
