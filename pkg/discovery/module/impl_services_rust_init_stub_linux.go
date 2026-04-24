// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !cgo

package module

// InitDiscoveryLogger is a no-op when CGO is not available.
func InitDiscoveryLogger() {}
