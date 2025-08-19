// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

// errNotSupported indicates that the current host doesn't fulfill the requirements for USM monitoring
type errNotSupported struct {
	error
}

// Unwrap returns the underlying error
func (e *errNotSupported) Unwrap() error {
	return e.error
}
