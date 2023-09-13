#ifndef HTTP2_H_
#define HTTP2_H_

#include "bpf_builtins.h"
#include "conn_tuple.h"

typedef struct {
    conn_tuple_t conn;
    void *buf;
    size_t len;
    size_t offset;
} http2_tls_info_t;

#endif // HTTP2_H_
