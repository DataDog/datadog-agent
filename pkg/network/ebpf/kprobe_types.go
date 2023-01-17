// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package ebpf

/*
#include "./c/tracer.h"
#include "./c/tcp_states.h"
#include "./c/prebuilt/offset-guess.h"
#include "./c/protocols/http2-defs.h"
*/
import "C"

type ConnTuple C.conn_tuple_t
type TCPStats C.tcp_stats_t
type ConnStats C.conn_stats_ts_t
type Conn C.conn_t
type Batch C.batch_t
type Telemetry C.telemetry_t
type PortBinding C.port_binding_t
type PIDFD C.pid_fd_t
type UDPRecvSock C.udp_recv_sock_t
type BindSyscallArgs C.bind_syscall_args_t

// udp_recv_sock_t have *sock and *msghdr struct members, we make them opaque here
type _Ctype_struct_sock uint64
type _Ctype_struct_msghdr uint64

type TCPState uint8

type StaticTableEnumKey = C.static_table_key_t

const (
	MethodKey StaticTableEnumKey = C.kMethod
	PathKey   StaticTableEnumKey = C.kPath
	StatusKey StaticTableEnumKey = C.kStatus
)

type StaticTableEnumValue = C.static_table_key_t

const (
	GetValue       StaticTableEnumValue = C.kGET
	PostValue      StaticTableEnumValue = C.kPOST
	EmptyPathValue StaticTableEnumValue = C.kEmptyPath
	IndexPathValue StaticTableEnumValue = C.kIndexPath
	K200Value      StaticTableEnumValue = C.k200
	K204Value      StaticTableEnumValue = C.k204
	K206Value      StaticTableEnumValue = C.k206
	K304Value      StaticTableEnumValue = C.k304
	K400Value      StaticTableEnumValue = C.k400
	K404Value      StaticTableEnumValue = C.k404
	K500Value      StaticTableEnumValue = C.k500
)

type StaticTableValue = C.static_table_entry_t

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

const BatchSize = C.CONN_CLOSED_BATCH_SIZE
