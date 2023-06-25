#ifndef __PROTOCOL_CLASSIFICATION_MAPS_H
#define __PROTOCOL_CLASSIFICATION_MAPS_H

#include "map-defs.h"

#include "protocols/classification/defs.h"
#include "protocols/classification/structs.h"

// Kernels before 4.7 do not know about per-cpu array maps.
#if defined(COMPILE_PREBUILT) || defined(COMPILE_CORE) || (defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0))

// A per-cpu buffer used to read requests fragments during protocol
// classification and avoid allocating a buffer on the stack. Some protocols
// requires us to read at offset that are not aligned. Such reads are forbidden
// if done on the stack and will make the verifier complain about it, but they
// are allowed on map elements, hence the need for this map.
BPF_PERCPU_ARRAY_MAP(classification_buf, __u32, char [CLASSIFICATION_MAX_BUFFER], 1)
#else
BPF_ARRAY_MAP(classification_buf, __u8, 1)
#endif

// A set (map from a key to a const bool value, we care only if the key exists in the map, and not its value) to
// mark if we've seen a specific mongo request, so we can eliminate false-positive classification on responses.
BPF_HASH_MAP(mongo_request_id, mongo_key, bool, 1024)

#endif
