#ifndef JAVA_MAPS_H
#define JAVA_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"
#include "tracer.h"

#include "java-tls-types.h"

// LINUX_VERSION_CODE doesn't work with co-re and is relevant to runtime compilation only
#ifdef COMPILE_RUNTIME

    // Kernels before 4.7 do not know about per-cpu array maps.
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0)
        // A per-cpu buffer used to read domain string and avoid allocating a buffer on the stack.
        // Domain name can be up to 64 bytes which sometimes hits the limit of 512bytes on the stack
        BPF_PERCPU_ARRAY_MAP(java_tls_hostname, __u32, connection_by_host_key_t, 1)
    #else
        // Kernels < 4.7.0 do not know about the per-cpu array map used
        // in classification, preventing the program to load even though
        // we won't use it. We change the type to a simple array map to
        // circumvent that.
        BPF_ARRAY_MAP(java_tls_hostname, __u32, 1)
    #endif

#else
    BPF_PERCPU_ARRAY_MAP(java_tls_hostname, __u32, connection_by_host_key_t, 1)
#endif

/* A set (map from a key to a const bool value, we care only if the key exists in the map, and not its value) to
   mark if we've seen a specific java tls connection.
   Map size is set to 1 as javaTLS is optional, this will be overwritten to MaxTrackedConnections
   if javaTLS is enabled. */
BPF_HASH_MAP(java_tls_connections, conn_tuple_t, bool, 1)

/* map to correlate peer domain and port with the actual conn_tuple
   Map size is set to 1 as javaTLS is optional, this will be overwritten to MaxTrackedConnections
   if javaTLS is enabled. */
BPF_HASH_MAP(java_conn_tuple_by_peer, connection_by_host_key_t, conn_tuple_t, 1)

#endif //JAVA_MAPS_H
