#ifndef __HTTP_MAPS_H
#define __HTTP_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "protocols/http/types.h"

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_HASH_MAP(http_in_flight, conn_tuple_t, http_transaction_t, 0)

#endif
