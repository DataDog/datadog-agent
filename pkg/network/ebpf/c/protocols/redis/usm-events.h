#ifndef __REDIS_USM_EVENTS_H
#define __REDIS_USM_EVENTS_H

#include "protocols/events.h"
#include "protocols/redis/types.h"

USM_EVENTS_INIT(redis, redis_event_t, REDIS_BATCH_SIZE);

#endif /* __REDIS_USM_EVENTS_H */
