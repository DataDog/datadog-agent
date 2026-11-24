// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// OnDemandNameOverrides is a set of overrides for on demand name field, which only supports `==`
var OnDemandNameOverrides = &eval.OpOverrides{
	EqualsOnly: true,
}
