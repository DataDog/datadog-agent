#ifndef __HTTP2_SKB_COMMON_H
#define __HTTP2_SKB_COMMON_H

// skip_preface is a helper function to check for the HTTP2 magic sent at the beginning
// of an HTTP2 connection, and skip it if present.
static __always_inline void skip_preface(const struct __sk_buff *skb, skb_info_t *skb_info) {
    char preface[HTTP2_MARKER_SIZE];
    bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
    bpf_skb_load_bytes(skb, skb_info->data_off, preface, HTTP2_MARKER_SIZE);
    if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
        skb_info->data_off += HTTP2_MARKER_SIZE;
    }
}

#endif // __HTTP2_SKB_COMMON_H
