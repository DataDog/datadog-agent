// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !darwin

package daemon

// loadClientID returns the remote config client ID the daemon must use. Only macOS keeps one
// across the daemons that replace one another during a configuration experiment: elsewhere the
// client gets a random ID, as it always did.
func loadClientID() string {
	return ""
}
