#ifndef __TAGS_H
#define __TAGS_H

#include "tracer.h"
#include "tracer-stats.h"
#include "tags-types.h"

// Static tags
static __always_inline void add_tags_stats(conn_stats_ts_t *stats, __u64 tags) {
    stats->tags |= tags;
}

#endif
