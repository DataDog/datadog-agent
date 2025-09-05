#ifndef __REDIS_MAPS_H
#define __REDIS_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "protocols/redis/types.h"

// Keeps track of in-flight Redis transactions
BPF_HASH_MAP(redis_in_flight, conn_tuple_t, redis_transaction_t, 0)

// Maps connection tuple to the current key name we're tracking.
BPF_HASH_MAP(redis_key_in_flight, conn_tuple_t, redis_key_data_t, 0)

// Acts as a scratch buffer for Redis events with keys, for preparing events before they are sent to userspace.
BPF_PERCPU_ARRAY_MAP(redis_with_key_scratch_buffer, redis_with_key_event_t, 1)

// Acts as a scratch buffer for Redis events, for preparing events before they are sent to userspace.
BPF_PERCPU_ARRAY_MAP(redis_scratch_buffer, redis_event_t, 1)

#endif /* __REDIS_MAPS_H */
