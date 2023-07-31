#ifndef __GRPC_DEFS_H
#define __GRPC_DEFS_H

typedef enum {
  PAYLOAD_UNDETERMINED,
  PAYLOAD_GRPC,
  PAYLOAD_NOT_GRPC,
} grpc_status_t;

#endif
