// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

// hasUnixSocketSupport reports whether a trace agent profiling socket can be
// reached on this platform.
//
// It is false on Windows, and not because the dial would fail to compile or to
// resolve: recent Windows builds implement AF_UNIX and Go dials it happily. It
// is false because no Windows Agent ever serves one. apm_config.receiver_socket
// has no default on Windows (defaultpaths.GetDefaultReceiverSocket returns ""),
// and fixupLinuxSockets -- the only code that fills it in -- runs on linux and
// aix alone (pkg/config/setup/fixup_init.go).
//
// What makes that worth guarding is how the failure would present. WithUDS only
// installs a dialer, so a path nothing serves is not an error at Start: it is a
// failed upload once per profiling period, forever, in the profiler's own logs.
// Worse, startForAgent treats a configured socket as a complete substitute for
// the local forwarding server and skips starting it, so the HTTP path that
// would otherwise have carried these profiles is gone too.
//
// Returning false costs a Windows host nothing it had: profiles take the HTTP
// transport they would have taken had the setting been absent.
func hasUnixSocketSupport() bool {
	return false
}
