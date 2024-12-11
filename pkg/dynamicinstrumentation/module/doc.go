// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package module encapsulates a system-probe module which uses uprobes and bpf
// to exfiltrate data from running processes. This is the Go implementation of
// the dynamic instrumentation product.
package module
