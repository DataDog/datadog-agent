#ifndef __POSTGRES_USM_EVENTS_H
#define __POSTGRES_USM_EVENTS_H

#include "protocols/events.h"
#include "protocols/postgres/types.h"

// Controls the number of Postgres transactions read from userspace at a time.
#define POSTGRES_BATCH_SIZE (MAX_BATCH_SIZE(postgres_event_t))

USM_EVENTS_INIT(postgres, postgres_event_t, POSTGRES_BATCH_SIZE);

#endif
