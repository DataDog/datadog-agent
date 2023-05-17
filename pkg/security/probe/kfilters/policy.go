// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"errors"
	"strings"
)

// PolicyMode represents the policy mode (accept or deny)
type PolicyMode uint8

// PolicyFlag is a bitmask of the active filtering policies
type PolicyFlag uint8

// Policy modes
const (
	PolicyModeNoFilter PolicyMode = iota
	PolicyModeAccept
	PolicyModeDeny
)

// Policy flags
const (
	PolicyFlagBasename PolicyFlag = 1
	PolicyFlagFlags    PolicyFlag = 2
	PolicyFlagMode     PolicyFlag = 4

	// need to be aligned with the kernel size
	BasenameFilterSize = 256
)

func (m PolicyMode) String() string {
	switch m {
	case PolicyModeAccept:
		return "accept"
	case PolicyModeDeny:
		return "deny"
	case PolicyModeNoFilter:
		return "no filter"
	}
	return ""
}

// MarshalJSON returns the JSON encoding of the policy mode
func (m PolicyMode) MarshalJSON() ([]byte, error) {
	s := m.String()
	if s == "" {
		return nil, errors.New("invalid policy mode")
	}

	return []byte(`"` + s + `"`), nil
}

// MarshalJSON returns the JSON encoding of the policy flags
func (f PolicyFlag) MarshalJSON() ([]byte, error) {
	var flags []string
	if f&PolicyFlagBasename != 0 {
		flags = append(flags, `"basename"`)
	}
	if f&PolicyFlagFlags != 0 {
		flags = append(flags, `"flags"`)
	}
	if f&PolicyFlagMode != 0 {
		flags = append(flags, `"mode"`)
	}
	return []byte("[" + strings.Join(flags, ",") + "]"), nil
}
