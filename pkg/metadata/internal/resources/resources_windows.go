// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package resources

// GetPayload builds a payload of processes metadata collected from gohai.
func GetPayload(hostname string) *Payload {
	// Not implemented on Windows
	return nil
}
