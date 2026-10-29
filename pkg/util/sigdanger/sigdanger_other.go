// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !aix

// Package sigdanger installs a handler for SIGDANGER, the AIX-specific signal
// the kernel sends when system paging space is nearly exhausted. On non-AIX
// platforms there is nothing to handle and Init is a no-op.
package sigdanger

// Init installs the SIGDANGER handler. It is a no-op on non-AIX platforms.
func Init() {}
