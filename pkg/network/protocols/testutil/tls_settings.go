// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package testutil provides general utilities for protocols UTs.
package testutil

// Constants to represent whether the connection should be encrypted with TLSEnabled.
const (
	TLSDisabled = false
	TLSEnabled  = true
)
