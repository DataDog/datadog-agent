// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kfilters holds kfilters related files
package kfilters

import (
	"encoding/json"
	"errors"
)

// PolicyMode represents the policy mode (accept or deny)
type PolicyMode uint8

// Policy modes
const (
	PolicyModeNoFilter PolicyMode = iota
	PolicyModeAccept
	PolicyModeDeny

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

	return json.Marshal(s)
}
