#ifndef __KAFKA_USM_EVENTS
#define __KAFKA_USM_EVENTS

#include "protocols/kafka/types.h"
#include "protocols/events.h"

// This controls the number of Kafka transactions read from userspace at a time
#define KAFKA_BATCH_SIZE (MAX_BATCH_SIZE(kafka_event_t))

USM_EVENTS_INIT(kafka, kafka_event_t, KAFKA_BATCH_SIZE);

#endif
