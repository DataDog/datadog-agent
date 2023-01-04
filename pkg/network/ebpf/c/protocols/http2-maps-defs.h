#ifndef __HTTP2_MAPS_DEFS_H
#define __HTTP2_MAPS_DEFS_H

BPF_HASH_MAP(http2_static_table, u64, static_table_value, 20)

BPF_HASH_MAP(http2_dynamic_table, u64, dynamic_table_value, 20)

BPF_HASH_MAP(http2_dynamic_counter_table, conn_tuple_t, u64, 10)

#endif
