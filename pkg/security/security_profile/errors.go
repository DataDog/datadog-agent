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

// ErrHostDumpDisabled is returned when a host-wide activity dump is requested but the feature is not enabled
var ErrHostDumpDisabled = errors.New("host activity dump is disabled: set runtime_security_config.activity_dump.host_dump.enabled to true")

// ErrHostDumpV2Unsupported is returned when a host-wide activity dump is requested while the V2 profile
// manager is active (which does not support the on-demand activity dump API)
var ErrHostDumpV2Unsupported = errors.New("host activity dump is not supported with the V2 security profile manager (runtime_security_config.security_profile.v2.enabled=true)")
