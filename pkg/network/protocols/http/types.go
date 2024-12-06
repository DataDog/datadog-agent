// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package http

/*
#include "../../ebpf/c/protocols/tls/tags-types.h"
#include "../../ebpf/c/protocols/http/types.h"
#include "../../ebpf/c/protocols/classification/defs.h"
*/
import "C"

type ConnTuple = C.conn_tuple_t
type SslSock C.ssl_sock_t
type SslReadArgs C.ssl_read_args_t

type EbpfEvent C.http_event_t
type EbpfTx C.http_transaction_t

const (
	BufferSize = C.HTTP_BUFFER_SIZE
)

type ConnTag = uint64

const (
	GnuTLS  ConnTag = C.LIBGNUTLS
	OpenSSL ConnTag = C.LIBSSL
	Go      ConnTag = C.GO
	TLS     ConnTag = C.CONN_TLS
	Istio   ConnTag = C.ISTIO
	NodeJS  ConnTag = C.NODEJS
)

var (
	StaticTags = map[ConnTag]string{
		GnuTLS:  "tls.library:gnutls",
		OpenSSL: "tls.library:openssl",
		Go:      "tls.library:go",
		TLS:     "tls.connection:encrypted",
		Istio:   "tls.library:istio",
		NodeJS:  "tls.library:nodejs",
	}
)
