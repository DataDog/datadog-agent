#ifndef __TAGS_TYPES_H
#define __TAGS_TYPES_H

// static tags limited to 64 tags per unique connection
enum static_tags {
    NO_TAGS = 0,
    LIBGNUTLS = (1<<0),
    LIBSSL = (1<<1),
    TLS = (1<<2),
};

#endif
