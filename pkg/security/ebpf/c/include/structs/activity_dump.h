#ifndef _STRUCTS_ACTIVITY_DUMP_H_
#define _STRUCTS_ACTIVITY_DUMP_H_

struct activity_dump_config {
    u64 event_mask;
    u64 timeout;
    u64 wait_list_timestamp;
    u64 start_timestamp;
    u64 end_timestamp;
    u16 events_rate;
    u16 padding;
    u32 paused;
};

#endif
