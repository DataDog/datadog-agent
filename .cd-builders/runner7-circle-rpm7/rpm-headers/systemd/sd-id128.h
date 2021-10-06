/*-*- Mode: C; c-basic-offset: 8; indent-tabs-mode: nil -*-*/

#ifndef foosdid128hfoo
#define foosdid128hfoo

/***
  This file is part of systemd.

  Copyright 2011 Lennart Poettering

  systemd is free software; you can redistribute it and/or modify it
  under the terms of the GNU Lesser General Public License as published by
  the Free Software Foundation; either version 2.1 of the License, or
  (at your option) any later version.

  systemd is distributed in the hope that it will be useful, but
  WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
  Lesser General Public License for more details.

  You should have received a copy of the GNU Lesser General Public License
  along with systemd; If not, see <http://www.gnu.org/licenses/>.
***/

#include <inttypes.h>
#include <string.h>

#include "_sd-common.h"

_SD_BEGIN_DECLARATIONS;

/* 128-bit ID APIs. See sd-id128(3) for more information. */

typedef union sd_id128 sd_id128_t;

union sd_id128 {
        uint8_t bytes[16];
        uint64_t qwords[2];
};

#define SD_ID128_STRING_MAX 33

char *sd_id128_to_string(sd_id128_t id, char s[SD_ID128_STRING_MAX]);

int sd_id128_from_string(const char *s, sd_id128_t *ret);

int sd_id128_randomize(sd_id128_t *ret);

int sd_id128_get_machine(sd_id128_t *ret);

int sd_id128_get_boot(sd_id128_t *ret);

#define SD_ID128_MAKE(v0, v1, v2, v3, v4, v5, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15) \
        ((const sd_id128_t) { .bytes = { 0x##v0, 0x##v1, 0x##v2, 0x##v3, 0x##v4, 0x##v5, 0x##v6, 0x##v7, \
                                   0x##v8, 0x##v9, 0x##v10, 0x##v11, 0x##v12, 0x##v13, 0x##v14, 0x##v15 }})

#define SD_ID128_ARRAY(v0, v1, v2, v3, v4, v5, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15) \
        { .bytes = { 0x##v0, 0x##v1, 0x##v2, 0x##v3, 0x##v4, 0x##v5, 0x##v6, 0x##v7, \
                     0x##v8, 0x##v9, 0x##v10, 0x##v11, 0x##v12, 0x##v13, 0x##v14, 0x##v15 }}

/* Note that SD_ID128_FORMAT_VAL will evaluate the passed argument 16
 * times. It is hence not a good idea to call this macro with an
 * expensive function as parameter or an expression with side
 * effects */

#define SD_ID128_FORMAT_STR "%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x"
#define SD_ID128_FORMAT_VAL(x) (x).bytes[0], (x).bytes[1], (x).bytes[2], (x).bytes[3], (x).bytes[4], (x).bytes[5], (x).bytes[6], (x).bytes[7], (x).bytes[8], (x).bytes[9], (x).bytes[10], (x).bytes[11], (x).bytes[12], (x).bytes[13], (x).bytes[14], (x).bytes[15]

#define SD_ID128_CONST_STR(x)                                           \
        ((const char[SD_ID128_STRING_MAX]) {                            \
                ((x).bytes[0] >> 4) >= 10 ? 'a' + ((x).bytes[0] >> 4) - 10 : '0' + ((x).bytes[0] >> 4), \
                ((x).bytes[0] & 15) >= 10 ? 'a' + ((x).bytes[0] & 15) - 10 : '0' + ((x).bytes[0] & 15), \
                ((x).bytes[1] >> 4) >= 10 ? 'a' + ((x).bytes[1] >> 4) - 10 : '0' + ((x).bytes[1] >> 4), \
                ((x).bytes[1] & 15) >= 10 ? 'a' + ((x).bytes[1] & 15) - 10 : '0' + ((x).bytes[1] & 15), \
                ((x).bytes[2] >> 4) >= 10 ? 'a' + ((x).bytes[2] >> 4) - 10 : '0' + ((x).bytes[2] >> 4), \
                ((x).bytes[2] & 15) >= 10 ? 'a' + ((x).bytes[2] & 15) - 10 : '0' + ((x).bytes[2] & 15), \
                ((x).bytes[3] >> 4) >= 10 ? 'a' + ((x).bytes[3] >> 4) - 10 : '0' + ((x).bytes[3] >> 4), \
                ((x).bytes[3] & 15) >= 10 ? 'a' + ((x).bytes[3] & 15) - 10 : '0' + ((x).bytes[3] & 15), \
                ((x).bytes[4] >> 4) >= 10 ? 'a' + ((x).bytes[4] >> 4) - 10 : '0' + ((x).bytes[4] >> 4), \
                ((x).bytes[4] & 15) >= 10 ? 'a' + ((x).bytes[4] & 15) - 10 : '0' + ((x).bytes[4] & 15), \
                ((x).bytes[5] >> 4) >= 10 ? 'a' + ((x).bytes[5] >> 4) - 10 : '0' + ((x).bytes[5] >> 4), \
                ((x).bytes[5] & 15) >= 10 ? 'a' + ((x).bytes[5] & 15) - 10 : '0' + ((x).bytes[5] & 15), \
                ((x).bytes[6] >> 4) >= 10 ? 'a' + ((x).bytes[6] >> 4) - 10 : '0' + ((x).bytes[6] >> 4), \
                ((x).bytes[6] & 15) >= 10 ? 'a' + ((x).bytes[6] & 15) - 10 : '0' + ((x).bytes[6] & 15), \
                ((x).bytes[7] >> 4) >= 10 ? 'a' + ((x).bytes[7] >> 4) - 10 : '0' + ((x).bytes[7] >> 4), \
                ((x).bytes[7] & 15) >= 10 ? 'a' + ((x).bytes[7] & 15) - 10 : '0' + ((x).bytes[7] & 15), \
                ((x).bytes[8] >> 4) >= 10 ? 'a' + ((x).bytes[8] >> 4) - 10 : '0' + ((x).bytes[8] >> 4), \
                ((x).bytes[8] & 15) >= 10 ? 'a' + ((x).bytes[8] & 15) - 10 : '0' + ((x).bytes[8] & 15), \
                ((x).bytes[9] >> 4) >= 10 ? 'a' + ((x).bytes[9] >> 4) - 10 : '0' + ((x).bytes[9] >> 4), \
                ((x).bytes[9] & 15) >= 10 ? 'a' + ((x).bytes[9] & 15) - 10 : '0' + ((x).bytes[9] & 15), \
                ((x).bytes[10] >> 4) >= 10 ? 'a' + ((x).bytes[10] >> 4) - 10 : '0' + ((x).bytes[10] >> 4), \
                ((x).bytes[10] & 15) >= 10 ? 'a' + ((x).bytes[10] & 15) - 10 : '0' + ((x).bytes[10] & 15), \
                ((x).bytes[11] >> 4) >= 10 ? 'a' + ((x).bytes[11] >> 4) - 10 : '0' + ((x).bytes[11] >> 4), \
                ((x).bytes[11] & 15) >= 10 ? 'a' + ((x).bytes[11] & 15) - 10 : '0' + ((x).bytes[11] & 15), \
                ((x).bytes[12] >> 4) >= 10 ? 'a' + ((x).bytes[12] >> 4) - 10 : '0' + ((x).bytes[12] >> 4), \
                ((x).bytes[12] & 15) >= 10 ? 'a' + ((x).bytes[12] & 15) - 10 : '0' + ((x).bytes[12] & 15), \
                ((x).bytes[13] >> 4) >= 10 ? 'a' + ((x).bytes[13] >> 4) - 10 : '0' + ((x).bytes[13] >> 4), \
                ((x).bytes[13] & 15) >= 10 ? 'a' + ((x).bytes[13] & 15) - 10 : '0' + ((x).bytes[13] & 15), \
                ((x).bytes[14] >> 4) >= 10 ? 'a' + ((x).bytes[14] >> 4) - 10 : '0' + ((x).bytes[14] >> 4), \
                ((x).bytes[14] & 15) >= 10 ? 'a' + ((x).bytes[14] & 15) - 10 : '0' + ((x).bytes[14] & 15), \
                ((x).bytes[15] >> 4) >= 10 ? 'a' + ((x).bytes[15] >> 4) - 10 : '0' + ((x).bytes[15] >> 4), \
                ((x).bytes[15] & 15) >= 10 ? 'a' + ((x).bytes[15] & 15) - 10 : '0' + ((x).bytes[15] & 15), \
                0 })

_sd_pure_ static inline int sd_id128_equal(sd_id128_t a, sd_id128_t b) {
        return memcmp(&a, &b, 16) == 0;
}

_sd_pure_ static inline int sd_id128_is_null(sd_id128_t a) {
        return a.qwords[0] == 0 && a.qwords[1] == 0;
}

#define SD_ID128_NULL ((const sd_id128_t) { .qwords = { 0, 0 }})

_SD_END_DECLARATIONS;

#endif
