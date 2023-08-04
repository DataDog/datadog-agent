// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package model

import (
	"strings"
)

// MarshalText maps the syscall identifier to UTF-8-encoded text and returns the result
func (s Syscall) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(strings.TrimPrefix(s.String(), "Sys"))), nil
}
