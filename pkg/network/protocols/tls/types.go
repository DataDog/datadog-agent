// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ignore

package tls

/*
#include "../../ebpf/c/protocols/tls/tags-types.h"
*/
import "C"

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
