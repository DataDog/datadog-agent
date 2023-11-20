#ifndef __HTTP2_USM_EVENTS_H
#define __HTTP2_USM_EVENTS_H

#include "protocols/http2/decoding-defs.h"
#include "protocols/events.h"

USM_EVENTS_INIT(http2, http2_event_t, HTTP2_BATCH_SIZE);

#endif
