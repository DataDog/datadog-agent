#ifndef __HTTP_USM_EVENTS_H
#define __HTTP_USM_EVENTS_H

#include "protocols/events.h"
#include "protocols/http/types.h"

USM_EVENTS_INIT(http, http_transaction_t, HTTP_BATCH_SIZE);

#endif
