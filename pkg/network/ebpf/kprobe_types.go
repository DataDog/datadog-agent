// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package ebpf

/*
#include "./c/pid_fd.h"
#include "./c/tracer/tracer.h"
#include "./c/tcp_states.h"
#include "./c/prebuilt/offset-guess.h"
#include "./c/protocols/classification/defs.h"
#include "./c/protocols/tls/tls-certs-types.h"

*/
import "C"

type ConnTuple C.conn_tuple_t
type TCPStats C.tcp_stats_t
type TCPCongestionStats C.tcp_congestion_stats_t
type TCPRTORecoveryStats C.tcp_rto_recovery_stats_t
type ConnStats C.conn_stats_ts_t
type Conn C.conn_t
type SkpConn C.skp_conn_tuple_t
type PidTs C.pid_ts_t
type Batch C.batch_t
type Telemetry C.telemetry_t
type PortBinding C.port_binding_t
type PIDFD C.pid_fd_t
type UDPRecvSock C.udp_recv_sock_t
type BindSyscallArgs C.bind_syscall_args_t
type ProtocolStack C.protocol_stack_t
type ProtocolStackWrapper C.protocol_stack_wrapper_t
type TLSTags C.tls_info_t
type TLSTagsWrapper C.tls_info_wrapper_t
type NetTimeMs C.time_ms_t
type CertItem C.cert_item_t
type CertSerial C.cert_serial_t
type CertDomain C.cert_domain_t
type CertValidity C.cert_validity_t
type SSLHandshakeState C.ssl_handshake_state_t

// udp_recv_sock_t have *sock and *msghdr struct members, we make them opaque here
type _Ctype_struct_sock uint64
type _Ctype_struct_msghdr uint64
type _Ctype_struct_sockaddr uint64

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

const BatchSize = C.CONN_CLOSED_BATCH_SIZE
const SizeofBatch = C.sizeof_batch_t

const SizeofConn = C.sizeof_conn_t

type ClassificationProgram = uint32
type ClassificationTLSProgram = uint32

const (
	ClassificationTLSClient ClassificationTLSProgram = C.CLASSIFICATION_TLS_CLIENT_PROG
	ClassificationTLSServer ClassificationTLSProgram = C.CLASSIFICATION_TLS_SERVER_PROG
	ClassificationQueues    ClassificationProgram    = C.CLASSIFICATION_QUEUES_PROG
	ClassificationDBs       ClassificationProgram    = C.CLASSIFICATION_DBS_PROG
	ClassificationGRPC      ClassificationProgram    = C.CLASSIFICATION_GRPC_PROG
)
