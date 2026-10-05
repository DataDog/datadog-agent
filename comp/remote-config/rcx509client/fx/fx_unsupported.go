// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin

// Package fx provides the Fx module for the Remote Config x509 client.
package fx

import "github.com/DataDog/datadog-agent/pkg/util/fxutil"

// Module is empty on platforms that the libdd-rc native client does not
// support. Keeping the no-op module lets the shared Remote Config bundle remain
// platform-independent.
func Module() fxutil.Module {
	return fxutil.Component()
}
