#ifndef __HTTP_USM_EVENTS_H
#define __HTTP_USM_EVENTS_H

#include "protocols/events.h"
#include "protocols/http/types.h"

// This controls the number of HTTP transactions read from userspace at a time
#define HTTP_BATCH_SIZE (MAX_BATCH_SIZE(http_event_t))

USM_EVENTS_INIT(http, http_event_t, HTTP_BATCH_SIZE);

#endif
