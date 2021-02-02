// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// NamedPipe interface to NamedPipes (multi-platform)
type NamedPipe interface {
	Open() error
	Ready() bool
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	Close() error
}
