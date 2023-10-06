#ifndef __TLS_MAPS_H
#define __TLS_MAPS_H

#include "map-defs.h"

#include "protocols/classification/defs.h"

BPF_PERCPU_ARRAY_MAP(tls_classification_heap, __u32, char[CLASSIFICATION_MAX_BUFFER], 1)

#endif
