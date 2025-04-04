// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package datadogagent implements the datadog agent install methods
package datadogagent

import "context"

// PostInstall runs post install scripts for a given package. Noop for Windows
func PostInstall(_ context.Context, _, _ string) error {
	return nil
}

// PreRemove runs pre remove scripts for a given package. Noop for Windows
func PreRemove(_ context.Context, _ string, _ string, _ bool) {}
