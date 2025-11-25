#ifndef __CONNECTION_CLOSE_EVENTS_H
#define __CONNECTION_CLOSE_EVENTS_H

#include "protocols/tls/connection-close.h"
#include "protocols/events.h"

#define TCP_CLOSE_BATCH_SIZE (MAX_BATCH_SIZE(tcp_close_event_t))

// Initialize batch events for connection close tracking
USM_EVENTS_INIT(tcp_close, tcp_close_event_t, TCP_CLOSE_BATCH_SIZE)

#endif // __CONNECTION_CLOSE_EVENTS_H