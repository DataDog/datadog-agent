#ifndef __CLASSIFIER_H
#define __CLASSIFIER_H

#include "tracer.h"

typedef struct {
    skb_info_t skb_info;
    conn_tuple_t tup;
} proto_args_t;

// this objects is emebedded in all protocol objects.
// it holds information needed in the classifier
// socket filter, before invoking the subprogram for
// handling a particular protocol.
typedef struct {
    __u8 done;
    __u8 failed;
} cnx_info_t;



#endif // __CLASSIFIER_H
