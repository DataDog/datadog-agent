// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerlifecycle constains constants used by the container life
// cycle check.
package containerlifecycle

const (
	// PayloadV1 represents the payload v1 version
	PayloadV1 = "v1"
	// EventNameDelete represents deletion events
	EventNameDelete = "delete"
	// ObjectKindContainer represents container events
	ObjectKindContainer = "container"
	// ObjectKindPod represents pod events
	ObjectKindPod = "pod"
)
