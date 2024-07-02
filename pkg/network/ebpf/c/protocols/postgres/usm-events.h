#ifndef __POSTGRES_USM_EVENTS_H
#define __POSTGRES_USM_EVENTS_H

#include "protocols/events.h"
#include "protocols/postgres/types.h"

USM_EVENTS_INIT(postgres, postgres_event_t, POSTGRES_BATCH_SIZE);

#endif
