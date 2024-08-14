#ifndef __POSTGRES_MAPS_H
#define __POSTGRES_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "protocols/postgres/types.h"

// Keeps track of in-flight Postgres transactions
BPF_HASH_MAP(postgres_in_flight, conn_tuple_t, postgres_transaction_t, 0)

// Acts as a scratch buffer for Postgres events, for preparing events before they are sent to userspace.
BPF_PERCPU_ARRAY_MAP(postgres_scratch_buffer, postgres_event_t, 1)

/* Postgres telemetry maps in kernel space help to find empirical statistics about
 * the number of Postgres messages in each TCP packet.
 * use only key 0, value is postgres telemetry object.
 */
BPF_ARRAY_MAP(postgres_plain_msg_count, postgres_kernel_msg_count_t, 1)
BPF_ARRAY_MAP(postgres_tls_msg_count, postgres_kernel_msg_count_t, 1)

#endif
