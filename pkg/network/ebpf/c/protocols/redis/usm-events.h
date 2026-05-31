#ifndef __REDIS_USM_EVENTS_H
#define __REDIS_USM_EVENTS_H

#include "protocols/direct_consumer.h"
#include "protocols/events.h"
#include "protocols/redis/types.h"

USM_EVENTS_INIT(redis_with_key, redis_with_key_event_t, REDIS_WITH_KEY_BATCH_SIZE);
USM_EVENTS_INIT(redis, redis_event_t, REDIS_BATCH_SIZE);

// Initialize DirectConsumer utilities for both Redis event streams (a single
// service_monitoring_config.redis.use_direct_consumer flag controls both).
USM_DIRECT_CONSUMER_INIT(redis_with_key, redis_with_key_event_t, redis_with_key_batch_events)
USM_DIRECT_CONSUMER_INIT(redis, redis_event_t, redis_batch_events)

// Returns true if any of the reids monitoring modes is enabled.
static __always_inline bool is_redis_enabled() {
    return is_redis_with_key_monitoring_enabled() || is_redis_monitoring_enabled();
}

#endif /* __REDIS_USM_EVENTS_H */
