//+build ignore

package ebpf

/*
#include "./c/runtime/conntrack-types.h"
*/
import "C"

type ConntrackTuple C.conntrack_tuple_t

type ConntrackTelemetry C.conntrack_telemetry_t
