// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rawpacket holds rawpacket related files
package rawpacket

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Filter defines a raw packet filter
type Filter struct {
	RuleID    eval.RuleID
	BPFFilter string
}
