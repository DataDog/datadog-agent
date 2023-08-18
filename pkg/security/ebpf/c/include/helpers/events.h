#ifndef _HELPERS_EVENTS_H
#define _HELPERS_EVENTS_H

#include "constants/enums.h"
#include "maps.h"

static __attribute__((always_inline)) u64 get_enabled_events(void) {
    u32 key = 0;
    u64 *mask = bpf_map_lookup_elem(&enabled_events, &key);
    if (mask) {
        return *mask;
    }
    return 0;
}

static __attribute__((always_inline)) int mask_has_event(u64 mask, enum event_type event) {
    return (mask & ((u64)1 << (u64)(event - EVENT_FIRST_DISCARDER))) != 0;
}

static __attribute__((always_inline)) int is_event_enabled(enum event_type event) {
    return mask_has_event(get_enabled_events(), event);
}

static __attribute__((always_inline)) void add_event_to_mask(u64 *mask, enum event_type event) {
    if (event == EVENT_ALL) {
        *mask = event;
    } else {
        *mask |= 1 << (event - EVENT_FIRST_DISCARDER);
    }
}

#endif
