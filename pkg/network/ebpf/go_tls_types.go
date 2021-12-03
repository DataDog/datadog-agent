//+build ignore

package ebpf

/*
#include "./c/go-tls-types.h"
*/
import "C"

type Location C.location_t
type SliceLocation C.slice_location_t
type GoroutineIDMetadata C.goroutine_id_metadata_t
type TlsConnLayout C.tls_conn_layout_t
type TlsProbeData C.tls_probe_data_t
