#ifndef __REDIS_USM_EVENTS_H
#define __REDIS_USM_EVENTS_H

#include "protocols/events.h"
#include "protocols/redis/types.h"

USM_EVENTS_INIT(redis_with_key, redis_with_key_event_t, REDIS_WITH_KEY_BATCH_SIZE);
USM_EVENTS_INIT(redis, redis_event_t, REDIS_BATCH_SIZE);

// Returns true if any of the reids monitoring modes is enabled.
static __always_inline bool is_redis_enabled() {
    return is_redis_with_key_monitoring_enabled() || is_redis_monitoring_enabled();
}

#endif /* __REDIS_USM_EVENTS_H */
