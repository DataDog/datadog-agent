#ifndef __TLS_CERTS_MAPS_H
#define __TLS_CERTS_MAPS_H

#include "map-defs.h"
#include "tls-certs-types.h"

BPF_HASH_MAP(ssl_certs_statem_args, __u64, void *, 1)
BPF_HASH_MAP(ssl_certs_i2d_X509_args, __u64, i2d_X509_args_t, 1)

BPF_HASH_MAP(ssl_handshake_state, void *, ssl_handshake_state_t, 1)
BPF_HASH_MAP(ssl_cert_info, cert_id_t, cert_item_t, 1)


#endif //__TLS_CERTS_MAPS_H
