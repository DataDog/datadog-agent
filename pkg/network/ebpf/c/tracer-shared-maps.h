#ifndef __TRACER_SHARED_MAPS_H
#define __TRACER_SHARED_MAPS_H

#include "map-defs.h"

// Maps a connection tuple to latest tcp segment we've processed. Helps to detect same packets that travels multiple
// interfaces or retransmissions.
BPF_HASH_MAP(connection_states, conn_tuple_t, u32, 0)

#endif
