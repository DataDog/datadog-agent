#ifndef __HTTP2_MAPS_DEFS_H
#define __HTTP2_MAPS_DEFS_H

// http2_static_table is the map that holding the supported static values by index and its static value.
BPF_HASH_MAP(http2_static_table, u64, static_table_entry_t, 20)

// http2_dynamic_table is the map that holding the supported dynamic values - the index is the static index and the
// tcp_con and it is value is the buffer which contains the dynamic string.
BPF_LRU_MAP(http2_dynamic_table, dynamic_table_index_t, dynamic_table_entry_t, 1024)

// http2_dynamic_counter_table is a map that holding the current dynamic values amount, in order to use for the
// internal calculation of the internal index in the http2_dynamic_table, it is hold by conn_tup to support different
// clients and the value is the current counter.
BPF_LRU_MAP(http2_dynamic_counter_table, conn_tuple_t, u64, 1024)

#endif
