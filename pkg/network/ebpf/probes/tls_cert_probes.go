// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package probes

const (
	// state machine probes -- within these functions, the TLS handshake can occur

	// SSLDoHandshakeProbe traces SSL_do_handshake
	SSLDoHandshakeProbe ProbeFuncName = "uprobe__SSL_do_handshake"
	// SSLDoHandshakeRetprobe traces the return of SSL_do_handshake
	SSLDoHandshakeRetprobe ProbeFuncName = "uretprobe__SSL_do_handshake"
	// SSLReadProbe traces SSL_read
	SSLReadProbe ProbeFuncName = "uprobe__SSL_read"
	// SSLReadRetprobe traces the return of SSL_read
	SSLReadRetprobe ProbeFuncName = "uretprobe__SSL_read"
	// SSLReadExProbe traces SSL_read_ex
	SSLReadExProbe ProbeFuncName = "uprobe__SSL_read_ex"
	// SSLReadExRetprobe traces the return of SSL_read_ex
	SSLReadExRetprobe ProbeFuncName = "uretprobe__SSL_read_ex"
	// SSLWriteProbe traces SSL_write
	SSLWriteProbe ProbeFuncName = "uprobe__SSL_write"
	// SSLWriteRetprobe traces the return of SSL_write
	SSLWriteRetprobe ProbeFuncName = "uretprobe__SSL_write"
	// SSLWriteExProbe traces SSL_write_ex
	SSLWriteExProbe ProbeFuncName = "uprobe__SSL_write_ex"
	// SSLWriteExRetprobe traces the return of SSL_write_ex
	SSLWriteExRetprobe ProbeFuncName = "uretprobe__SSL_write_ex"

	// cert serialization probes -- these catch the actual certificate data

	// I2DX509Probe traces i2d_X509
	I2DX509Probe ProbeFuncName = "uprobe__i2d_X509"
	// I2DX509Retprobe traces the return of i2d_X509
	I2DX509Retprobe ProbeFuncName = "uretprobe__i2d_X509"

	// cleanup probes

	// RawTracepointSchedProcessExit traces process exit to clean up old pids from maps
	RawTracepointSchedProcessExit ProbeFuncName = "raw_tracepoint__sched_process_exit_ssl_cert"
)

const (
	// SSLCertsStatemArgsMap is the map storing the SSL handle,
	// for functions that enter the SSL state machine
	SSLCertsStatemArgsMap BPFMapName = "ssl_certs_statem_args"

	// SSLCertsI2DX509ArgsMap stores the out parameter that i2d_X509 writes its result to
	SSLCertsI2DX509ArgsMap BPFMapName = "ssl_certs_i2d_X509_args"

	// SSLHandshakeStateMap associates an SSL handle with certificate data and a kernel socket.
	SSLHandshakeStateMap BPFMapName = "ssl_handshake_state"

	// SSLCertInfoMap associates cert IDs with certificate data
	SSLCertInfoMap BPFMapName = "ssl_cert_info"
)
