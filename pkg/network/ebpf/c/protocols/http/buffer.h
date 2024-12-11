#ifndef __HTTP_BUFFER_H
#define __HTTP_BUFFER_H

#include "ktypes.h"
#if defined(COMPILE_PREBUILT) || defined(COMPILE_RUNTIME)
#include <linux/err.h>
#endif

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "protocols/http/types.h"
#include "protocols/read_into_buffer.h"

READ_INTO_USER_BUFFER(http, HTTP_BUFFER_SIZE)
READ_INTO_USER_BUFFER(classification, CLASSIFICATION_MAX_BUFFER)

READ_INTO_BUFFER(skb, HTTP_BUFFER_SIZE, BLK_SIZE)

#endif
