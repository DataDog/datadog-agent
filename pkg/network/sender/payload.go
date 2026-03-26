// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import "net/http"

type result struct {
	payloads []payload
	size     int64
}

type payload struct {
	body    []byte
	headers http.Header
}

// Weight implements WeightedItem
func (r result) Weight() int64 {
	return r.size
}

// Type implements WeightedItem
func (r result) Type() string {
	return "connections"
}
