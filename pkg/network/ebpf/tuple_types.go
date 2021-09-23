//+build ignore

package ebpf

/*
#include "./c/tracer.h"
*/
import "C"

type ConnType uint32

const (
	UDP ConnType = C.CONN_TYPE_UDP
	TCP ConnType = C.CONN_TYPE_TCP
)

type ConnFamily uint32

const (
	IPv4 ConnFamily = C.CONN_V4
	IPv6 ConnFamily = C.CONN_V6
)

type ConnDirection uint8

const (
	Unknown  ConnDirection = C.CONN_DIRECTION_UNKNOWN
	Incoming ConnDirection = C.CONN_DIRECTION_INCOMING
	Outgoing ConnDirection = C.CONN_DIRECTION_OUTGOING
)
