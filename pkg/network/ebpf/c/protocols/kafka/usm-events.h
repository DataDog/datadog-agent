#ifndef __KAFKA_USM_EVENTS
#define __KAFKA_USM_EVENTS

#include "protocols/kafka/types.h"
#include "protocols/events.h"

USM_EVENTS_INIT(kafka, kafka_transaction_batch_entry_t, KAFKA_BATCH_SIZE);

#endif
