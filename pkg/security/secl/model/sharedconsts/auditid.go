// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedconsts holds model related shared constants
package sharedconsts

import "math"

const (
	// AuditUIDUnset is used to specify that a login uid is not set
	AuditUIDUnset = math.MaxUint32
)
