#ifndef __USM_DEBUG
#define __USM_DEBUG

#include "conn_tuple.h"

#ifndef LOAD_CONSTANT
#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" : "=r"(var))
#endif

#ifndef STRINGIFY
#define STRINGIFY(x) #x
#endif

#ifndef TO_STRING
#define TO_STRING(x) STRINGIFY(x)
#endif

#define process_filter(tup, param)                         \
    do {                                                   \
        u64 has = (u64)(tup)->param;                       \
        u64 want = 0;                                      \
        LOAD_CONSTANT(TO_STRING(filter_##param), want);    \
        if (want && want != has) {                         \
            return false;                                  \
        }                                                  \
    } while(0)                                             \


// this is used in DEBUG mode as a filter for HTTP requests
static __always_inline bool usm_should_process(conn_tuple_t *tup) {
    process_filter(tup, sport);
    process_filter(tup, dport);
    process_filter(tup, saddr_h);
    process_filter(tup, saddr_l);
    process_filter(tup, daddr_h);
    process_filter(tup, daddr_l);
    return true;
}
#endif
