#ifndef __TLS_CERTS_PARSER_H
#define __TLS_CERTS_PARSER_H

#ifndef TEST_BUILD_NO_EBPF

#include "map-defs.h"

#else

#include <stdio.h>
#include <stdbool.h>
#include <string.h>
#include <sys/types.h>

#define log_debug(format, ...) printf(format "\n", ##__VA_ARGS__)
#define bpf_memcmp(a, b, sz) memcmp(a, b, sz)

#define u8 unsigned char
#define s32 int
#define u32 unsigned int

static __always_inline long bpf_probe_read_user(void *dst, u32 size, const void *unsafe_ptr) {
    memcpy(dst, unsafe_ptr, size);
    return 0;
}

#endif


// RFC 5280 states that serial numbers can't be longer than 20 bytes
#define MAX_SERIAL_LEN 20
// technically domains can be longer than this...
#define DOMAIN_LEN 64

// UTC time length including zulu:
// YYMMDDhhmmssZ
#define UTC_ZULU_LEN 13
// UTC time without the Z at the end
#define UTC_TIME_LEN 12
typedef struct {
    u8 not_before[UTC_TIME_LEN];
    u8 not_after[UTC_TIME_LEN];
} cert_validity_t;

typedef struct {
    u8 len;
    u8 data[MAX_SERIAL_LEN];
} cert_serial_t;

typedef struct {
    u8 len;
    u8 data[DOMAIN_LEN];
} cert_domain_t;

typedef struct {
    bool is_ca;

    cert_serial_t serial;

    cert_domain_t domain;

    cert_validity_t validity;
} cert_t;

const int foo = sizeof(cert_t);


typedef struct {
    u8 *buf;
    u8 *end;
} data_t;
const static data_t null_data;


static __always_inline size_t data_size(data_t data) {
    return data.end - data.buf;
}
static __always_inline bool is_data_consumed(data_t data) {
    return data.buf == data.end;
}


static __always_inline bool data_peek(void *target, data_t *data, u32 size) {
    u8 *next = data->buf + size;

    if (next > data->end) {
        log_debug("data_peek tried to read %d bytes, which is past the end of the data", size);
        return true;
    }
    int err = bpf_probe_read_user(target, size, data->buf);
    if (err) {
        log_debug("bpf_probe_read_user failed to read %d bytes, error=%d", size, err);
        return true;
    }

    return false;
}
static __always_inline bool data_read(void *target, data_t *data, u32 size) {
    u8 *next = data->buf + size;

    if (next > data->end) {
        log_debug("data_read tried to read %d bytes, which is past the end of the data", size);
        return true;
    }
    int err = bpf_probe_read_user(target, size, data->buf);
    if (err) {
        log_debug("bpf_probe_read_user failed to read %d bytes, error=%d", size, err);
        return true;
    }

    data->buf = next;
    return false;
}


static __always_inline s32 read_elem_size(data_t *data) {
    u8 meta_size = 0;
    if (data_read(&meta_size, data, 1)) {
        log_debug("read_elem_size failed to read the meta_size byte");
        return -1;
    }

    if (meta_size < 128) {
        return meta_size;
    }

    // size >= 128 means we use "long form" length encoding
    meta_size -= 128;
    // no need to consider anything larger than 3 bytes
    const int MAX_BYTES = 3;
    if (meta_size > MAX_BYTES) {
        log_debug("read_elem_size got a byte length which is too large: %d", meta_size);
        return -1;
    }

    s32 retval = 0;
    for (int i = 0; i < MAX_BYTES; i++) {
        if (i >= meta_size) {
            break;
        }
        retval <<= 8;

        u8 digit = 0;
        if (data_read(&digit, data, 1)) {
            log_debug("read_elem_size failed to read length[%d]", i);
            return -1;
        }
        retval += digit;
    }
    return retval;
}

static const u8 BOOL_TYPE = 1;
static const u8 INT_TYPE = 0x02;
static const u8 BIT_STR_TYPE = 0x03;
static const u8 OCTET_STR_TYPE = 0x04;
static const u8 OBJECT_ID_TYPE = 0x06;
static const u8 UTC_DATE_TYPE = 0x17;
static const u8 SEQ_TYPE = 0x30;
static const u8 CONTEXT_SPECIFIC_TYPE = 0xa0;

static __always_inline data_t expect_der_elem(data_t *data, u8 expected_type) {
    u8 actual_type = 0;
    if (data_read(&actual_type, data, 1)) {
        log_debug("expect_der_elem failed to read type byte");
        return null_data;
    }
    if (expected_type != actual_type) {
        log_debug("expect_der_elem wanted %02x for type, got %02x (pointer: %p)", expected_type, actual_type, data->buf);
        return null_data;
    }

    s32 size = read_elem_size(data);
    if (size < 0) {
        log_debug("expect_der_elem failed to read_elem_size");
        return null_data;
    }

    data_t retval = { data->buf, data->buf + size };
    if (retval.end > data->end) {
        log_debug("expect_der_elem found a length that was too big");
        return null_data;
    }
    data->buf = retval.end;

    return retval;
}




static __always_inline bool parse_cert_version(data_t *data) {
    data_t outer_version = expect_der_elem(data, CONTEXT_SPECIFIC_TYPE | 0);
    if (!outer_version.buf) {
        log_debug("parse_cert_version failed to get outer_version");
        return true;
    }
    data_t inner_version = expect_der_elem(&outer_version, INT_TYPE);
    if (!inner_version.buf) {
        log_debug("parse_cert_version failed to get inner_version");
        return true;
    }
    size_t size = data_size(inner_version);
    if (size != 1) {
        log_debug("parse_cert_version saw unexpected inner_version size %zd", size);
        return true;
    }
    u8 version = 0;
    if (data_read(&version, &inner_version, 1)) {
        log_debug("parse_cert_version failed to read version");
        return true;
    }

    if (version != 2) {
        log_debug("parse_cert_version expected a version of 2, but got %d, bailing", version);
        return true;
    }

    return false;
}


static __always_inline bool parse_cert_date(data_t *data, u8 *dst) {
    data_t utc_data = expect_der_elem(data, UTC_DATE_TYPE);
    if (!utc_data.buf) {
        log_debug("parse_cert_date failed to find utc data");
        return true;
    }

    size_t size = data_size(utc_data);
    if (size != UTC_ZULU_LEN) {
        log_debug("parse_cert_date size was wrong, expected %d but got %zd bytes", UTC_ZULU_LEN, size);
        return true;
    }

    // read all of it except for the Z at the end
    if (data_read(dst, &utc_data, UTC_TIME_LEN)) {
        log_debug("parse_cert_date failed to copy utc data");
        return true;
    }

    return false;
}

static __always_inline bool parse_cert_serial(data_t *data, cert_t *cert) {
    data_t serial_int = expect_der_elem(data, INT_TYPE);
    if (!serial_int.buf) {
        log_debug("parse_cert_serial failed to read int");
        return true;
    }

    size_t size = data_size(serial_int);
    if (size > 20) {
        log_debug("parse_cert_serial expected <= 20 bytes but got %zd", size);
        return true;
    }
    
    cert->serial.len = size;
    if (data_read(&cert->serial.data, &serial_int, size)) {
        log_debug("parse_cert_serial failed to read serial data");
        return true;
    }

    return false;
}


static __always_inline bool parse_cert_validity(data_t *data, cert_t *cert) {
    data_t validity_seq = expect_der_elem(data, SEQ_TYPE);
    if (!validity_seq.buf) {
        log_debug("parse_cert_validity failed to read validity seq");
        return true;
    }

    if (parse_cert_date(&validity_seq, (u8*) &cert->validity.not_before)) {
        log_debug("parse_cert_validity failed to parse not_before");
        return true;
    }

    if (parse_cert_date(&validity_seq, (u8*) &cert->validity.not_after)) {
        log_debug("parse_cert_validity failed to parse not_after");
        return true;
    }

    return false;
}

static __always_inline bool parse_key_usage(data_t *data, cert_t *cert) {
    // data_t usage_seq = 
    data_t usage_bitstr = expect_der_elem(data, BIT_STR_TYPE);
    if (!usage_bitstr.buf) {
        log_debug("parse_key_usage failed to get usage_bitstr");
        return true;
    }
    
    size_t size = data_size(usage_bitstr);
    if (size < 2) {
        log_debug("parse_key_usage expected at least 2 bytes for usage_bitstr, got %zd", size);
        return true;
    }

    u8 extra_bits = 0;
    if (data_read(&extra_bits, &usage_bitstr, 1)) {
        log_debug("parse_key_usage failed to read extra_bits");
        return true;
    }
    u8 set_bits = (size - 1) * 8 - extra_bits;

    u8 usage_bits = 0;
    if (data_read(&usage_bits, &usage_bitstr, 1)) {
        log_debug("parse_key_usage failed to read usage_bits");
        return true;
    }

    // based off RFC 2459, section 4.2.1.3 -- Key Usage,
    // we know keyCertSign is bit #5, and it's MSB first
    const u8 CA_BIT = 5;
    // bits are zero indexed, so we start from 7
    const u8 CA_MASK = 1 << (7 - CA_BIT);
    cert->is_ca = set_bits >= CA_BIT && (usage_bits & CA_MASK);

    return false;
}


static __always_inline bool parse_alternative_names(data_t *data, cert_t *cert) {
    data_t alt_name_seq = expect_der_elem(data, SEQ_TYPE);
    if (!alt_name_seq.buf) {
        log_debug("parse_alternative_names failed to get alt_name_seq");
        return true;
    }

    for (int i = 0; i < 16; i++) {
        if (is_data_consumed(alt_name_seq)) {
            break;
        }

        u8 next_type = 0;
        if (data_peek(&next_type, &alt_name_seq, 1)) {
            log_debug("parse_alternative_names failed to read next_type");
            return true;
        }

        data_t name = expect_der_elem(&alt_name_seq, next_type);
        if (!name.buf) {
            log_debug("parse_alternative_names failed to get name");
            return true;
        }

        // this is context specific and thus not applicable elsewhere
        const u8 DNS_NAME_TYPE = 0x82;
        if (next_type != DNS_NAME_TYPE) {
            continue;
        }

        u8 domain_len = DOMAIN_LEN;
        size_t size = data_size(name);
        if (size < DOMAIN_LEN) {
            domain_len = size;
        }

        cert->domain.len = domain_len;
        if (data_read(&cert->domain.data, &name, domain_len)) {
            log_debug("parse_alternative_names failed to copy domain");
            return true;
        }

        // we found a domain, break out
        break;
    }

    return false;
}


#define SUBJECT_ALT_NAME_ID "\x55\x1D\x11"
#define KEY_USAGE_ID "\x55\x1D\x0F"

static __always_inline bool parse_cert_extensions(data_t *data, cert_t *cert) {
    data_t extensions = expect_der_elem(data, CONTEXT_SPECIFIC_TYPE | 3);
    if (!extensions.buf) {
        log_debug("parse_cert_extensions failed to get extensions");
        return true;
    }

    data_t extensions_seq = expect_der_elem(&extensions, SEQ_TYPE);
    if (!extensions.buf) {
        log_debug("parse_cert_extensions failed to get extensions_seq");
        return true;
    }


    data_t key_usage_value = {0};
    data_t alt_name_value = {0};
    for (int i = 0; i < 32; i++) {
        if (is_data_consumed(extensions_seq)) {
            break;
        }
        data_t single_ext_seq = expect_der_elem(&extensions_seq, SEQ_TYPE);
        if (!single_ext_seq.buf) {
            log_debug("parse_cert_extensions failed to get single_ext_seq on extension %d", i);
            return true;
        }

        data_t obj_id = expect_der_elem(&single_ext_seq, OBJECT_ID_TYPE);
        if (!obj_id.buf) {
            log_debug("parse_cert_extensions failed to get obj_id on extension %d", i);
            return true;
        }

        u8 next_type = 0;
        if (data_peek(&next_type, &single_ext_seq, 1)) {
            log_debug("parse_cert_extensions failed to read next_type on extension %d", i);
            return true;
        }
        if (next_type == BOOL_TYPE) {
            // if they added the "critical" boolean, skip
            if (!expect_der_elem(&single_ext_seq, BOOL_TYPE).buf) {
                log_debug("parse_cert_extensions failed to read critical bool on extension %d", i);
                return true;
            }
        }

        data_t extension_value = expect_der_elem(&single_ext_seq, OCTET_STR_TYPE);
        if (!extension_value.buf) {
            log_debug("parse_cert_extensions failed to read extension_value on extension %d", i);
            return true;
        }

        // the IDs we care about are all length 3
        if (data_size(obj_id) != 3) {
            continue;
        }

        char obj_id_buf[3] = {0};
        if (data_read(obj_id_buf, &obj_id, 3)) {
            log_debug("parse_cert_extensions failed to copy obj_id on extension %d", i);
            return true;
        }

        if (!bpf_memcmp(KEY_USAGE_ID, obj_id_buf, 3)) {
            key_usage_value = extension_value;
        } else if (!bpf_memcmp(SUBJECT_ALT_NAME_ID, obj_id_buf, 3)) {
            alt_name_value = extension_value;
        }
    }

    if (!is_data_consumed(extensions_seq)) {
        log_debug("parse_cert_extensions saw too many extensions");
        return true;
    }


    if (key_usage_value.buf && parse_key_usage(&key_usage_value, cert)) {
        log_debug("parse_cert_extensions failed to parse_key_usage");
        return true;
    }

    if (alt_name_value.buf && parse_alternative_names(&alt_name_value, cert)) {
        log_debug("parse_cert_extensions failed to parse_alternative_names");
        return true;
    }

    return false;
}



static __always_inline bool parse_tbs_cert(data_t *data, cert_t *cert) {
    data_t tbs_cert_seq = expect_der_elem(data, SEQ_TYPE);
    if (!tbs_cert_seq.buf) {
        log_debug("parse_tbs_cert failed to read sequence");
        return true;
    }

    if (parse_cert_version(&tbs_cert_seq)) {
        log_debug("parse_tbs_cert failed to parse_cert_version");
        return true;
    }

    if (parse_cert_serial(&tbs_cert_seq, cert)) {
        log_debug("parse_tbs_cert failed to parse_cert_serial");
        return true;
    }

    // we don't care about the signature's algorithm, skip it
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_debug("parse_tbs_cert failed to read algorithm seq");
        return true;
    }
    // issuer -- also irrelevant
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_debug("parse_tbs_cert failed to read issuer seq");
        return true;
    }

    if (parse_cert_validity(&tbs_cert_seq, cert)) {
        log_debug("parse_tbs_cert failed to parse_cert_validity");
        return true;
    }

    // subject -- also irrelevant
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_debug("parse_tbs_cert failed to read subject seq");
        return true;
    }

    // subject public key -- also irrelevant
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_debug("parse_tbs_cert failed to read subject public key seq");
        return true;
    }

    // issuerUniqueID and subjectUniqueID come next, but they are
    // IMPLICIT OPTIONAL, long deprecated and never seen

    if (parse_cert_extensions(&tbs_cert_seq, cert)) {
        log_debug("parse_tbs_cert failed to parse_cert_extensions");
        return true;
    }
    
    return false;
}

static __always_inline bool parse_cert(data_t data, cert_t *cert) {
    data_t cert_seq = expect_der_elem(&data, SEQ_TYPE);
    if (!cert_seq.buf) {
        log_debug("parse_cert failed to get sequence");
        return true;
    }

    return parse_tbs_cert(&cert_seq, cert);
}


#endif //__TLS_CERTS_PARSER_H

