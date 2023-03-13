// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package http

/*
#include "../../ebpf/c/tracer.h"
#include "../../ebpf/c/protocols/tls/tags-types.h"
#include "../../ebpf/c/protocols/http/types.h"
#include "../../ebpf/c/protocols/classification/defs.h"
*/
import "C"

type httpConnTuple = C.conn_tuple_t
type sslSock C.ssl_sock_t
type sslReadArgs C.ssl_read_args_t

type ebpfHttpTx C.http_transaction_t

type libPath C.lib_path_t

type ProtocolType C.protocol_t

// Add tests to TestProtocolValue
const (
	ProtocolUnknown  ProtocolType = C.PROTOCOL_UNKNOWN
	ProtocolHTTP     ProtocolType = C.PROTOCOL_HTTP
	ProtocolHTTP2    ProtocolType = C.PROTOCOL_HTTP2
	ProtocolKafka    ProtocolType = C.PROTOCOL_KAFKA
	ProtocolTLS      ProtocolType = C.PROTOCOL_TLS
	ProtocolMONGO    ProtocolType = C.PROTOCOL_MONGO
	ProtocolPostgres ProtocolType = C.PROTOCOL_POSTGRES
	ProtocolAMQP     ProtocolType = C.PROTOCOL_AMQP
	ProtocolRedis    ProtocolType = C.PROTOCOL_REDIS
	ProtocolMySQL    ProtocolType = C.PROTOCOL_MYSQL
	ProtocolMax      ProtocolType = C.MAX_PROTOCOLS
)

type DispatcherProgramType C.dispatcher_prog_t

const (
	DispatcherKafkaProg DispatcherProgramType = C.DISPATCHER_KAFKA_PROG
)

const (
	HTTPBufferSize = C.HTTP_BUFFER_SIZE

	libPathMaxSize = C.LIB_PATH_MAX_SIZE
)

type ConnTag = uint64

const (
	GnuTLS  ConnTag = C.LIBGNUTLS
	OpenSSL ConnTag = C.LIBSSL
	Go      ConnTag = C.GO
	Java    ConnTag = C.JAVA_TLS
)

var (
	StaticTags = map[ConnTag]string{
		GnuTLS:  "tls.library:gnutls",
		OpenSSL: "tls.library:openssl",
		Go:      "tls.library:go",
		Java:    "tls.library:java",
	}
)
