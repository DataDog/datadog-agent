// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package http

func getPathBufferSize(c *config.Config) int {
	return int(HTTP_BUFFER_SIZE)
}

func getMaxPathBufferSize(b []byte) int {
	return int(HTTP_BUFFER_SIZE)
}
