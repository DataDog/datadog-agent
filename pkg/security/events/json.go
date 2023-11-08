// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

// EventMarshaler defines an abstract json marshaller
type EventMarshaler interface {
	ToJSON() ([]byte, error)
}
