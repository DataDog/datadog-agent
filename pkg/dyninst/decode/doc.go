// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package decode is responsible for decoding raw event bytes from dynamic
// instrumentation's ebpf ringbuffer into JSON to be uploaded to the datadog
// logs backend.
package decode
