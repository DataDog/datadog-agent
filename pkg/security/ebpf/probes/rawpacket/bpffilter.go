// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rawpacket holds rawpacket related files
package rawpacket

import (
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// TCAct is the type of the tc action
type TCAct int

const (
	// TCActOk will terminate the packet processing pipeline and allows the packet to proceed
	TCActOk TCAct = 0
	// TCActShot will terminate the packet processing pipeline and drop the packet
	TCActShot TCAct = 2
	// TCActUnspec will continue packet processing
	TCActUnspec TCAct = -1
)

// Policy defines the policy for a raw packet filter
type Policy int

const (
	// PolicyAllow allows the packet to pass
	PolicyAllow Policy = iota
	// PolicyDrop drops the packet
	PolicyDrop
)

// ToTCAct converts a policy to a TCAct
func (p Policy) ToTCAct() TCAct {
	switch p {
	case PolicyDrop:
		return TCActShot
	default:
		return TCActUnspec
	}
}

// String returns the string representation of the policy
func (p Policy) String() string {
	switch p {
	case PolicyDrop:
		return "drop"
	default:
		return "allow"
	}
}

// Parse parses a string and sets the policy
func (p *Policy) Parse(str string) {
	switch str {
	case "drop":
		*p = PolicyDrop
	default:
		*p = PolicyAllow
	}
}

// Filter defines a raw packet filter
type Filter struct {
	RuleID        eval.RuleID
	BPFFilter     string
	Policy        Policy
	Pid           uint32
	CGroupPathKey model.PathKey
}

// Key returns a key representing the filter
func (f *Filter) Key() string {
	return f.RuleID + ":" + strconv.FormatUint(uint64(f.Pid), 10) + ":" + strconv.FormatUint(f.CGroupPathKey.Inode, 10)
}
