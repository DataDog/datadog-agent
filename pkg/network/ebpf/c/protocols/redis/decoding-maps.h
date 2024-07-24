#ifndef __REDIS_MAPS_H
#define __REDIS_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "protocols/redis/types.h"

// Keeps track of in-flight Redis transactions
BPF_HASH_MAP(redis_in_flight, conn_tuple_t, redis_transaction_t, 0)

#endif /* __REDIS_MAPS_H */
