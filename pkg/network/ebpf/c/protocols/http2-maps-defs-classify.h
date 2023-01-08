#ifndef __HTTP2_MAPS_DEFS_CLASSIFY_H
#define __HTTP2_MAPS_DEFS_CLASSIFY_H

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_LRU_MAP(http2_in_flight, conn_tuple_t, http2_transaction_t, 0)

#endif
