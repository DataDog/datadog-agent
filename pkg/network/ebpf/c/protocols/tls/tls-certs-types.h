#ifndef __TLS_CERTS_TYPES_H
#define __TLS_CERTS_TYPES_H

// macros to split pid_tgid to pid and tgid
#define PID_FROM(pid_tgid) ((__u32) pid_tgid)
#define TGID_FROM(pid_tgid) ((__u32) (pid_tgid >> 32))

#ifndef TEST_BUILD_NO_EBPF

#include "ktypes.h"

#else

#include <stdbool.h>

typedef unsigned char __u8;
typedef int __s32;
typedef unsigned int __u32;
typedef long long unsigned int __u64;

#endif


typedef __u32 cert_id_t;

// RFC 5280 states that serial numbers can't be longer than 20 bytes
#define MAX_SERIAL_LEN 20
// technically alt names can be longer than this, but common names are limited to 64
#define DOMAIN_LEN 64

// UTC time length including Z for zulu:
// YYMMDDhhmmssZ
#define UTC_ZONE_LEN 13
// UTC time without the Z at the end
#define UTC_ZONELESS_LEN 12
typedef struct {
    __u8 not_before[UTC_ZONELESS_LEN];
    __u8 not_after[UTC_ZONELESS_LEN];
} cert_validity_t;

typedef struct {
    __u8 len;
    __u8 data[MAX_SERIAL_LEN];
} cert_serial_t;

typedef struct {
    __u8 len;
    __u8 data[DOMAIN_LEN];
} cert_domain_t;


typedef struct {
    cert_id_t cert_id;

    cert_serial_t serial;
    cert_domain_t domain;
    cert_validity_t validity;
    bool is_ca;
} cert_t;

typedef struct {
    __u64 timestamp;

    cert_serial_t serial;
    cert_domain_t domain;
    cert_validity_t validity;
} cert_item_t;

typedef struct {
    cert_item_t cert_item;
    cert_id_t cert_id;
} ssl_handshake_state_t;

typedef struct {
    __u8 **out;
    // i2d_X509 has two behaviors:
    // 1. if *out is NULL, it will allocate a new buffer for the output
    // 2. if *out is not NULL, it will use the buffer pointed to by *out, AND overwrite the pointer so
    //    that it points past the end of what it wrote
    // out_deref stores *out so we can handle these cases
    __u8 *out_deref;
} i2d_X509_args_t;


#endif //__TLS_CERTS_TYPES_H
