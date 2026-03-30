#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_builtins.h"

#include "maps.h"

__maybe_unused static __always_inline __u64 get_ringbuf_flags(size_t data_size) {
    __u64 ringbuffer_wakeup_size = 0;
    LOAD_CONSTANT("ringbuffer_wakeup_size", ringbuffer_wakeup_size);
    if (ringbuffer_wakeup_size == 0) {
        return 0;
    }

    __u64 sz = bpf_ringbuf_query(&conn_close_event, DD_BPF_RB_AVAIL_DATA);
    return (sz + data_size) >= ringbuffer_wakeup_size ? DD_BPF_RB_FORCE_WAKEUP : DD_BPF_RB_NO_WAKEUP;
}

#include "sk_tcp.h"
#include "sk_udp.h"

//#include "tracer/udp_recv.h"
//#include "tracer/udp_send.h"
//#include "tracer/udp.h"
//#include "tracer/udpv6.h"
//#include "tracer/classifier.h"
//#include "tracer/inet.h"
//#include "protocols/tls/tls-certs.h"

char _license[] SEC("license") = "GPL";
