#ifndef __TLS_CERTS_PARSER_H
#define __TLS_CERTS_PARSER_H


#include "tls-certs-types.h"
#ifndef TEST_BUILD_NO_EBPF

#include "ktypes.h"

// there are enough places where log_bail is called that enabling it causes verifier trouble
#define log_bail()

#else

#include <stdio.h>
#include <stdbool.h>
#include <string.h>
#include <sys/types.h>

#define log_debug(format, ...) printf(format "\n", ##__VA_ARGS__)
#define log_bail() log_debug("certs parser bailed in func %s line %d\n", __func__, __LINE__)
#define bpf_memcmp(a, b, sz) memcmp(a, b, sz)
#define barrier_var(a)

static __always_inline long bpf_probe_read_user(void *dst, __u32 size, const void *unsafe_ptr) {
    memcpy(dst, unsafe_ptr, size);
    return 0;
}

#endif


typedef struct {
    __u8 *buf;
    __u8 *end;
} data_t;


static __always_inline __u32 data_size(data_t data) {
    return data.end - data.buf;
}
static __always_inline bool is_data_consumed(data_t data) {
    return data.buf >= data.end;
}

static __always_inline bool data_peek_impl(void *target, data_t *data, __u32 sizeof_target, __u32 size) {
    if (data->buf + size > data->end) {
        log_bail();
        return true;
    }
    // llvm will optimize out our bounds checks, failing the verifier, unless we use volatile
    volatile __u32 vol_size = size;
    __u32 checked_size = vol_size;
    if (checked_size > sizeof_target) {
        log_bail();
        return true;
    }

    int err = bpf_probe_read_user(target, checked_size, data->buf);
    if (err) {
        log_bail();
        return true;
    }

    return false;
}
static __always_inline bool data_read_impl(void *target, data_t *data, __u32 sizeof_target, __u32 size) {
    if (data_peek_impl(target, data, sizeof_target, size)) {
        log_bail();
        return true;
    }

    data->buf += size;
    return false;
}

#define data_peek(target, data, size) data_peek_impl((target), (data), sizeof(*target), size)
#define data_read(target, data, size) data_read_impl((target), (data), sizeof(*target), size)


static __always_inline __s32 read_elem_size(data_t *data) {
    // no need to consider sizes larger than 3 bytes, plus 1 byte for meta size
    __u8 size_buf[4] = {0};

    __u32 size_cap = data_size(*data);
    if (size_cap > sizeof(size_buf)) {
        size_cap = sizeof(size_buf);
    }
    if (data_peek(&size_buf, data, size_cap)) {
        log_bail();
        return -1;
    }
    data->buf++;

    __u8 meta_size = size_buf[0];
    if (meta_size < 128) {
        return meta_size;
    }

    // size >= 128 means we use "long form" length encoding
    meta_size -= 128;
    __u8 actual_size = meta_size + 1;
    if (actual_size > size_cap) {
        log_bail();
        return -1;
    }

    // this is a hand unrolled big endian decoding because the compiler couldn't figure out how
    __s32 retval = 0;
    __u8 *cursor = &size_buf[1];
    switch (meta_size) {
        case 3:
            retval <<= 8;
            retval += *cursor++;
            // passthrough
        case 2:
            retval <<= 8;
            retval += *cursor++;
            // passthrough
        case 1:
            retval <<= 8;
            retval += *cursor++;
            break;
        default:
            log_bail();
            return -1;
    }
    data->buf += meta_size;

    return retval;
}

#define BOOL_TYPE 0x01
#define INT_TYPE 0x02
#define BIT_STR_TYPE 0x03
#define OCTET_STR_TYPE 0x04
#define OBJECT_ID_TYPE 0x06
#define UTC_DATE_TYPE 0x17
#define SEQ_TYPE 0x30
#define CONTEXT_SPECIFIC_TYPE 0xa0

static __always_inline data_t expect_der_elem(data_t *data, __u8 expected_type) {
    __u8 actual_type = 0;
    data_t null_data = {0};
    if (data_read(&actual_type, data, 1)) {
        log_bail();
        return null_data;
    }
    if (expected_type != actual_type) {
        log_bail();
        return null_data;
    }

    __s32 size = read_elem_size(data);
    if (size < 0) {
        log_bail();
        return null_data;
    }

    data_t retval = { data->buf, data->buf + size };
    if (retval.end > data->end) {
        log_bail();
        return null_data;
    }
    data->buf = retval.end;

    return retval;
}


static __always_inline bool parse_cert_version(data_t *data) {
    data_t outer_version = expect_der_elem(data, CONTEXT_SPECIFIC_TYPE | 0);
    if (!outer_version.buf) {
        log_bail();
        return true;
    }
    data_t inner_version = expect_der_elem(&outer_version, INT_TYPE);
    if (!inner_version.buf) {
        log_bail();
        return true;
    }
    if (data_size(inner_version) != 1) {
        log_bail();
        return true;
    }
    __u8 version = 0;
    if (data_read(&version, &inner_version, 1)) {
        log_bail();
        return true;
    }

    if (version != 2) {
        log_bail();
        return true;
    }

    return false;
}



static __always_inline bool parse_cert_serial(data_t *data, cert_t *cert) {
    data_t serial_int = expect_der_elem(data, INT_TYPE);
    if (!serial_int.buf) {
        log_bail();
        return true;
    }

    __u32 size = data_size(serial_int);
    if (size > 20) {
        log_bail();
        return true;
    }
    
    cert->serial.len = size;
    if (data_read(&cert->serial.data, &serial_int, size)) {
        log_bail();
        return true;
    }

    return false;
}

static __always_inline bool parse_cert_date(data_t *data, __u8 (*dst)[UTC_ZONELESS_LEN]) {
    data_t utc_data = expect_der_elem(data, UTC_DATE_TYPE);
    if (!utc_data.buf) {
        log_bail();
        return true;
    }

    if (data_size(utc_data) != UTC_ZONE_LEN) {
        log_bail();
        return true;
    }

    // read all of it except for the Z at the end
    if (data_read(dst, &utc_data, UTC_ZONELESS_LEN)) {
        log_bail();
        return true;
    }

    return false;
}

static __always_inline bool parse_cert_validity(data_t *data, cert_t *cert) {
    data_t validity_seq = expect_der_elem(data, SEQ_TYPE);
    if (!validity_seq.buf) {
        log_bail();
        return true;
    }

    if (parse_cert_date(&validity_seq, &cert->validity.not_before)) {
        log_bail();
        return true;
    }

    if (parse_cert_date(&validity_seq, &cert->validity.not_after)) {
        log_bail();
        return true;
    }

    return false;
}

static __always_inline bool parse_key_usage(data_t *data, cert_t *cert) {
    data_t usage_bitstr = expect_der_elem(data, BIT_STR_TYPE);
    if (!usage_bitstr.buf) {
        log_bail();
        return true;
    }
    
    __u32 size = data_size(usage_bitstr);
    if (size < 2) {
        log_bail();
        return true;
    }

    __u8 extra_bits = 0;
    if (data_read(&extra_bits, &usage_bitstr, 1)) {
        log_bail();
        return true;
    }
    __u8 set_bits = (size - 1) * 8 - extra_bits;

    __u8 usage_bits = 0;
    if (data_read(&usage_bits, &usage_bitstr, 1)) {
        log_bail();
        return true;
    }

    // based off RFC 2459, section 4.2.1.3 -- Key Usage,
    // we know keyCertSign is bit #5, and it's MSB first
    const __u8 CA_BIT = 5;
    // bits are zero indexed, so we start from 7
    const __u8 CA_MASK = 1 << (7 - CA_BIT);
    cert->is_ca = set_bits >= CA_BIT && (usage_bits & CA_MASK);

    return false;
}

static __always_inline bool parse_domain(data_t *data, cert_t *cert) {
    __u8 next_type = 0;
    if (data_peek(&next_type, data, 1)) {
        log_bail();
        return true;
    }

    data_t name = expect_der_elem(data, next_type);
    if (!name.buf) {
        log_bail();
        return true;
    }

    // this is context specific and thus not applicable elsewhere
    const __u8 DNS_NAME_TYPE = 0x82;
    if (next_type != DNS_NAME_TYPE) {
        return false;
    }

    __u8 domain_len = DOMAIN_LEN;
    __u32 size = data_size(name);
    if (size < DOMAIN_LEN) {
        domain_len = size;
    }

    cert->domain.len = domain_len;
    if (data_read(&cert->domain.data, &name, domain_len)) {
        log_bail();
        return true;
    }

    return false;
}



static __always_inline bool parse_alternative_names(data_t *data, cert_t *cert) {
    data_t alt_name_seq = expect_der_elem(data, SEQ_TYPE);
    if (!alt_name_seq.buf) {
        log_bail();
        return true;
    }

    for (int i = 0; i < 8; i++) {
        if (is_data_consumed(alt_name_seq)) {
            break;
        }

        if (parse_domain(&alt_name_seq, cert)) {
            log_bail();
            return true;
        }
        // if we found a domain, stop searching
        if (cert->domain.len) {
            break;
        }
    }

    return false;
}

#define SUBJECT_ALT_NAME_ID "\x55\x1D\x11"
#define KEY_USAGE_ID "\x55\x1D\x0F"

static __always_inline bool parse_single_extension(data_t *data, data_t *key_usage_value, data_t *alt_name_value) {
    data_t single_ext_seq = expect_der_elem(data, SEQ_TYPE);
    if (!single_ext_seq.buf) {
        log_bail();
        return true;
    }

    data_t obj_id = expect_der_elem(&single_ext_seq, OBJECT_ID_TYPE);
    if (!obj_id.buf) {
        log_bail();
        return true;
    }

    __u8 next_type = 0;
    if (data_peek(&next_type, &single_ext_seq, 1)) {
        log_bail();
        return true;
    }
    if (next_type == BOOL_TYPE) {
        // if they added the "critical" boolean, skip
        if (!expect_der_elem(&single_ext_seq, BOOL_TYPE).buf) {
            log_bail();
            return true;
        }
    }

    data_t extension_value = expect_der_elem(&single_ext_seq, OCTET_STR_TYPE);
    if (!extension_value.buf) {
        log_bail();
        return true;
    }

    // the IDs we care about are all length 3
    if (data_size(obj_id) != 3) {
        return false;
    }

    char obj_id_buf[3] = {0};
    if (data_read(&obj_id_buf, &obj_id, 3)) {
        log_bail();
        return true;
    }

    if (!bpf_memcmp(KEY_USAGE_ID, obj_id_buf, 3)) {
        *key_usage_value = extension_value;
    } else if (!bpf_memcmp(SUBJECT_ALT_NAME_ID, obj_id_buf, 3)) {
        *alt_name_value = extension_value;
    }

    return false;
}


static __always_inline bool parse_cert_extensions(data_t *data, cert_t *cert) {
    data_t extensions = expect_der_elem(data, CONTEXT_SPECIFIC_TYPE | 3);
    if (!extensions.buf) {
        log_bail();
        return true;
    }

    data_t extensions_seq = expect_der_elem(&extensions, SEQ_TYPE);
    if (!extensions.buf) {
        log_bail();
        return true;
    }


    data_t key_usage_value = {0};
    data_t alt_name_value = {0};

    for (int i = 0; i < 24; i++) {
        if (is_data_consumed(extensions_seq)) {
            break;
        }
        if (parse_single_extension(&extensions_seq, &key_usage_value, &alt_name_value)) {
            log_bail();
            return true;
        }
    }


    if (!is_data_consumed(extensions_seq)) {
        log_bail();
        return true;
    }


    if (key_usage_value.buf && parse_key_usage(&key_usage_value, cert)) {
        log_bail();
        return true;
    }

    if (alt_name_value.buf && parse_alternative_names(&alt_name_value, cert)) {
        log_bail();
        return true;
    }

    return false;
}


static __always_inline bool parse_tbs_cert(data_t *data, cert_t *cert) {
    data_t tbs_cert_seq = expect_der_elem(data, SEQ_TYPE);
    if (!tbs_cert_seq.buf) {
        log_bail();
        return true;
    }

    if (parse_cert_version(&tbs_cert_seq)) {
        log_bail();
        return true;
    }

    if (parse_cert_serial(&tbs_cert_seq, cert)) {
        log_bail();
        return true;
    }

    // we don't care about the signature's algorithm, skip it
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_bail();
        return true;
    }
    // issuer -- also irrelevant
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_bail();
        return true;
    }

    if (parse_cert_validity(&tbs_cert_seq, cert)) {
        log_bail();
        return true;
    }

    // subject -- also irrelevant
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_bail();
        return true;
    }

    // subject public key -- also irrelevant
    if (!expect_der_elem(&tbs_cert_seq, SEQ_TYPE).buf) {
        log_bail();
        return true;
    }

    // issuerUniqueID and subjectUniqueID come next, but they are
    // IMPLICIT OPTIONAL, long deprecated and never seen

    if (parse_cert_extensions(&tbs_cert_seq, cert)) {
        log_bail();
        return true;
    }
    
    return false;
}


#define SIG_CHUNKS 8
static __always_inline bool parse_signature(data_t *data, cert_t *cert) {
    // algorithm -- irrelevant
    if (!expect_der_elem(data, SEQ_TYPE).buf) {
        log_debug("no seq");
        log_bail();
        return true;
    }

    data_t sig_bitstr = expect_der_elem(data, BIT_STR_TYPE);
    if (!sig_bitstr.buf) {
        log_debug("no bitstr");
        log_bail();
        return true;
    }
    // skip the first byte which indicates how many bits are missing
    sig_bitstr.buf++;

    // turn the signature (a source of random bits) into a unique-enough UUID,
    // by XOR'ing the signature togther.
    __u32 xor_total = 0;
    __u32 chunks[SIG_CHUNKS] = {0};

    __u32 to_copy = data_size(sig_bitstr) / sizeof(__u32);
    __u32 to_copy_bytes = to_copy * sizeof(__u32);
    if (to_copy_bytes > sizeof(chunks)) {
        to_copy_bytes = sizeof(chunks);
        to_copy = SIG_CHUNKS;
    }
    if (to_copy == 0) {
        log_bail();
        return true;
    }

    if (data_read(&chunks, &sig_bitstr, to_copy_bytes)) {
        log_bail();
        return true;
    }

    for (int i = 0; i < SIG_CHUNKS; i++) {
        xor_total ^= chunks[i];
    }

    cert->cert_id = xor_total;

    return false;
}

static __always_inline bool parse_cert(data_t data, cert_t *cert) {
    data_t cert_seq = expect_der_elem(&data, SEQ_TYPE);
    if (!cert_seq.buf) {
        log_bail();
        return true;
    }

    if (parse_tbs_cert(&cert_seq, cert)) {
        log_bail();
        return true;
    }

    if (parse_signature(&cert_seq, cert)) {
        log_bail();
        return true;
    }

    return false;
}


#endif //__TLS_CERTS_PARSER_H

