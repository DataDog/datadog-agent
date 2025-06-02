// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import "errors"

// ErrActivityDumpManagerDisabled is returned when the activity dump manager is disabled
var ErrActivityDumpManagerDisabled = errors.New("ActivityDumpManager is disabled")

// ErrSecurityProfileManagerDisabled is returned when the security profile manager is disabled
var ErrSecurityProfileManagerDisabled = errors.New("SecurityProfileManager is disabled")
