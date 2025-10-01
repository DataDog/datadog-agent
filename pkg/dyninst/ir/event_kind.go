// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

//go:generate go run golang.org/x/tools/cmd/stringer -type=EventKind -trimprefix=EventKind -output event_kind_string.go

// EventKind is the kind of event.
type EventKind uint8

const (
	_ EventKind = iota

	// EventKindEntry is an event that emits an entry.
	EventKindEntry
	// EventKindReturn is an event that emits a return.
	EventKindReturn

	maxEventKind uint8 = iota
)

// IsValid returns true if the event kind is valid.
func (k EventKind) IsValid() bool {
	return k > 0 && uint8(k) < maxEventKind
}
