#ifndef __TAGS_TYPES_H
#define __TAGS_TYPES_H

// static tags limited to 64 tags per unique connection
enum static_tags {
    NO_TAGS = 0,
    LIBGNUTLS = (1<<0),
    LIBSSL = (1<<1),
    GO = (1<<2),
    JAVA_TLS = (1<<3),
    CONN_TLS = (1<<4),
    ISTIO = (1<<5),
    NODEJS = (1<<6),
};

#endif
