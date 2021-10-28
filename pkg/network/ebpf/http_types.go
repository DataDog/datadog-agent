//+build ignore

package ebpf

/*
#include "./c/tracer.h"
#include "./c/http-types.h"
*/
import "C"

type HTTPConnTuple C.conn_tuple_t
type HTTPBatchState C.http_batch_state_t
type SSLSock C.ssl_sock_t
type SSLReadArgs C.ssl_read_args_t
