#ifndef __TAGS_H
#define __TAGS_H

#include "tracer-conn-stats.h"
#include "tags-types.h"

// Static tags
static __always_inline void add_tags_stats(conn_stats_ts_t *stats, __u64 tags) {
    stats->tags |= tags;
}

static __always_inline void add_tags_tuple(conn_tuple_t *t, __u64 tags) {
    conn_stats_ts_t *stats = get_conn_stats(t);
    if (!stats) {
        return;
    }
    add_tags_stats(stats, tags);
}
#endif
