#ifndef __KAFKA_MAPS_H
#define __KAFKA_MAPS_H

#include "map-defs.h"

#include "protocols/kafka/defs.h"
#include "protocols/kafka/types.h"

BPF_PERCPU_ARRAY_MAP(kafka_client_id, char [CLIENT_ID_SIZE_TO_VALIDATE], 1)
BPF_PERCPU_ARRAY_MAP(kafka_topic_name, char [TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE], 1)

#endif
