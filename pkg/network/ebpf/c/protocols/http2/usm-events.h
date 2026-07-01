#ifndef __HTTP2_USM_EVENTS_H
#define __HTTP2_USM_EVENTS_H

#include "protocols/http2/decoding-defs.h"
#include "protocols/events.h"

USM_EVENTS_INIT(http2, http2_event_t, HTTP2_BATCH_SIZE);

USM_EVENTS_INIT(terminated_http2, conn_tuple_t, HTTP2_TERMINATED_BATCH_SIZE);

// Initialize DirectConsumer utilities for HTTP2 protocol
USM_DIRECT_CONSUMER_INIT(http2, http2_event_t, http2_batch_events)

#endif
