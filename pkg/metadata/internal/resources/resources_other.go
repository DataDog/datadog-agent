// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly

package resources

// GetPayload currently just a stub.
func GetPayload(hostname string) *Payload {

	//unimplemented for misc platforms.
	return &Payload{}
}
