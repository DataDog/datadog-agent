//+build ignore

package ebpf

/*
#include "./c/tracer.h"
#include "./c/tcp_states.h"
#include "./c/prebuilt/offset-guess.h"
*/
import "C"

type ConnTuple C.conn_tuple_t
type TCPStats C.tcp_stats_t
type ConnStats C.conn_stats_ts_t
type Conn C.conn_t
type Batch C.batch_t
type Telemetry C.telemetry_t
type PortBinding C.port_binding_t

type TCPState uint8

const (
	Established TCPState = C.TCP_ESTABLISHED
	Close       TCPState = C.TCP_CLOSE
)

type ConnFlags uint32

const (
	LInit   ConnFlags = C.CONN_L_INIT
	RInit   ConnFlags = C.CONN_R_INIT
	Assured ConnFlags = C.CONN_ASSURED
)

type PortState uint8

const (
	PortListening PortState = C.PORT_LISTENING
	PortClosed    PortState = C.PORT_CLOSED
)

const BatchSize = C.CONN_CLOSED_BATCH_SIZE
