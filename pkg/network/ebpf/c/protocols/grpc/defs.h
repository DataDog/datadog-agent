#ifndef __GRPC_DEFS_H
#define __GRPC_DEFS_H

typedef enum {
    PAYLOAD_UNDETERMINED,
    PAYLOAD_GRPC,
    PAYLOAD_NOT_GRPC,
} grpc_status_t;

typedef struct {
    __u32 offset;
    __u32 length;
} frame_info_t;

#endif
