#ifndef __USM_CONTEXT_H
#define __USM_CONTEXT_H

#include "tracer/tracer.h"
#include "protocols/classification/common.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/stack-helpers.h"

// from uapi/linux/if_packet.h
#define PACKET_OUTGOING 4

typedef struct {
    char data[CLASSIFICATION_MAX_BUFFER];
    size_t size;
} classification_buffer_t;

typedef struct {
    struct __sk_buff *owner;
    conn_tuple_t tuple;
    skb_info_t  skb_info;
    classification_buffer_t buffer;
    // bit mask with layers that should be skiped
    u16 routing_skip_layers;
    classification_prog_t routing_current_program;
} usm_context_t;

// Kernels before 4.7 do not know about per-cpu array maps.
#if defined(COMPILE_PREBUILT) || defined(COMPILE_CORE) || (defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0))

// A per-cpu buffer used to read requests fragments during protocol
// classification and avoid allocating a buffer on the stack. Some protocols
// requires us to read at offset that are not aligned. Such reads are forbidden
// if done on the stack and will make the verifier complain about it, but they
// are allowed on map elements, hence the need for this map.
//
// Why do we have 2 map entries per CPU?
// This has to do with the way socket-filters are executed.
//
// It's possible for a socket-filter program to be preempted by a softirq and
// replaced by another program from the *opposite* network direction. In other
// words, there is a chance that ingress and egress packets can be processed
// concurrently on the same CPU, which is why have a dedicated per CPU map entry
// for each direction in order to avoid data corruption.
BPF_PERCPU_ARRAY_MAP(classification_buf, usm_context_t, 2)
#else
BPF_ARRAY_MAP(classification_buf, __u8, 1)
#endif

static __always_inline usm_context_t* __get_usm_context(struct __sk_buff *skb) {
    // we use the packet direction as the key to the CPU map
    const u32 key = skb->pkt_type == PACKET_OUTGOING;
    return bpf_map_lookup_elem(&classification_buf, &key);
}

static __always_inline void __init_buffer(struct __sk_buff *skb, skb_info_t *skb_info, classification_buffer_t* buffer) {
    bpf_memset(buffer->data, 0, sizeof(buffer->data));
    read_into_buffer_for_classification((char *)buffer->data, skb, skb_info->data_off);
    const size_t payload_length = skb->len - skb_info->data_off;
    buffer->size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
}

static __always_inline usm_context_t* usm_context_init(struct __sk_buff *skb, conn_tuple_t *tuple, skb_info_t *skb_info) {
    if (!skb || !skb_info) {
        return NULL;
    }

    usm_context_t *usm_context = __get_usm_context(skb);
    if (!usm_context) {
        return NULL;
    }

    usm_context->owner = skb;
    usm_context->tuple = *tuple;
    usm_context->skb_info = *skb_info;
    __init_buffer(skb, skb_info, &usm_context->buffer);
    return usm_context;
}

static __always_inline usm_context_t* usm_context(struct __sk_buff *skb) {
    usm_context_t *usm_context = __get_usm_context(skb);
    if (!usm_context) {
        return NULL;
    }

    // sanity check
    if (usm_context->owner != skb) {
        log_debug("invalid usm context");
        return NULL;
    }

    return usm_context;
}

#endif
